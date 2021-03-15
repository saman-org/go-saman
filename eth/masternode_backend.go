// Copyright 2018 The go-saman Authors
// This file is part of the go-saman library.
//
// The go-saman library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-saman library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-saman library. If not, see <http://www.gnu.org/licenses/>.

package eth

import (
	"bytes"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
	"errors"
	"context"

	"github.com/saman-org/go-saman/common"
	"github.com/saman-org/go-saman/contracts/masternode/contract"
	"github.com/saman-org/go-saman/core/types"
	"github.com/saman-org/go-saman/core/types/masternode"
	"github.com/saman-org/go-saman/event"
	"github.com/saman-org/go-saman/log"
	"github.com/saman-org/go-saman/p2p"
	"github.com/saman-org/go-saman/params"
	"github.com/saman-org/go-saman/p2p/discover"
	"crypto/ecdsa"
	"github.com/saman-org/go-saman/crypto"
	"github.com/saman-org/go-saman/eth/downloader"
	"github.com/saman-org/go-saman"
)

var (
	statsReportInterval  = 10 * time.Second // Time interval to report vote pool stats
	ErrUnknownMasternode = errors.New("unknown masternode")
)

type MasternodeManager struct {
	// channels for fetcher, syncer, txsyncLoop
	IsMasternode uint32
	srvr         *p2p.Server
	contract     *contract.Contract

	mux *event.TypeMux
	eth *Ethereum

	syncing int32

	mu          sync.RWMutex
	ID          string
	NodeAccount common.Address
	PrivateKey  *ecdsa.PrivateKey
}

func NewMasternodeManager(eth *Ethereum) (*MasternodeManager, error) {
	contractBackend := NewContractBackend(eth)
	contract, err := contract.NewContract(params.MasterndeContractAddress, contractBackend)
	if err != nil {
		return nil, err
	}
	// Create the masternode manager with its initial settings
	manager := &MasternodeManager{
		eth:                eth,
		contract:           contract,
		syncing:            0,
	}
	return manager, nil
}

func (self *MasternodeManager) Clear() {
	self.mu.Lock()
	defer self.mu.Unlock()

}

func (self *MasternodeManager) Start(srvr *p2p.Server, mux *event.TypeMux) {
	self.srvr = srvr
	self.mux = mux
	log.Trace("MasternodeManqager start ")
	x8 := srvr.Self().X8()
	self.ID = fmt.Sprintf("%x", x8[:])
	self.NodeAccount = crypto.PubkeyToAddress(srvr.Config.PrivateKey.PublicKey)
	self.PrivateKey = srvr.Config.PrivateKey

	go self.masternodeLoop()
	go self.checkSyncing()
}

func (self *MasternodeManager) Stop() {

}

func (self *MasternodeManager) masternodeLoop() {
	xy := self.srvr.Self().XY()
	has, err := self.contract.Has(nil, self.srvr.Self().X8())
	if err != nil {
		log.Error("contract.Has", "error", err)
	}
	if has {
		fmt.Println("### It's already been a masternode! ")
		atomic.StoreUint32(&self.IsMasternode, 1)
	} else {
		atomic.StoreUint32(&self.IsMasternode, 0)
		if self.srvr.IsMasternode {
			data := "0x2f926732" + common.Bytes2Hex(xy[:])
			fmt.Printf("### Masternode Transaction Data: %s\n", data)
		}
	}

	joinCh := make(chan *contract.ContractJoin, 32)
	quitCh := make(chan *contract.ContractQuit, 32)
	joinSub, err1 := self.contract.WatchJoin(nil, joinCh)
	if err1 != nil {
		// TODO: exit
		return
	}
	quitSub, err2 := self.contract.WatchQuit(nil, quitCh)
	if err2 != nil {
		// TODO: exit
		return
	}

	ping := time.NewTimer(masternode.MASTERNODE_PING_INTERVAL)
	defer ping.Stop()
	ntp := time.NewTimer(time.Second)
	defer ntp.Stop()

	report := time.NewTicker(statsReportInterval)
	defer report.Stop()

	for {
		select {
		case join := <-joinCh:
			if bytes.Equal(join.Id[:], xy[:]) {
				atomic.StoreUint32(&self.IsMasternode, 1)
				fmt.Println("### Become a masternode! ")
			}
		case quit := <-quitCh:
			if bytes.Equal(quit.Id[:], xy[0:8]) {
				atomic.StoreUint32(&self.IsMasternode, 0)
				fmt.Println("### Remove a masternode! ")
			}
		case err := <-joinSub.Err():
			joinSub.Unsubscribe()
			fmt.Println("eventJoin err", err.Error())
		case err := <-quitSub.Err():
			quitSub.Unsubscribe()
			fmt.Println("eventQuit err", err.Error())

		case <-ntp.C:
			ntp.Reset(10 * time.Minute)
			go discover.CheckClockDrift()
		case <-ping.C:
			ping.Reset(masternode.MASTERNODE_PING_INTERVAL)
			if atomic.LoadUint32(&self.IsMasternode) == 0 {
				has, err := self.contract.Has(nil, self.srvr.Self().X8())
				if has && err == nil {
					fmt.Println("### Set masternode flag")
					atomic.StoreUint32(&self.IsMasternode, 1)
				}else{
					continue
				}
			}
			logTime := time.Now().Format("2006-01-02 15:04:05")
			if atomic.LoadInt32(&self.syncing) == 1 {
				fmt.Println(logTime, " syncing...")
				break
			}
			address := self.NodeAccount
			stateDB, _ := self.eth.blockchain.State()
			if stateDB.GetBalance(address).Cmp(big.NewInt(1e+16)) < 0 {
				fmt.Println(logTime, "Failed to deposit 0.01 saman to ", address.String())
				break
			}
			gasPrice, err := self.eth.APIBackend.gpo.SuggestPrice(context.Background())
			if err != nil {
				fmt.Println(logTime, "Get gas price error:", err)
				gasPrice = big.NewInt(20e+9)
			}
			msg := ethereum.CallMsg{From: address, To: &params.MasterndeContractAddress}
			contractBackend := NewContractBackend(self.eth)
			gas, err := contractBackend.EstimateGas(context.Background(), msg)
			if err != nil {
				fmt.Println("Get gas error:", err)
				continue
			}
			minPower := new(big.Int).Mul(big.NewInt(int64(gas)), gasPrice)
			fmt.Println(logTime, "gasPrice ", gasPrice.String(), "minPower ", minPower.String())
			if stateDB.GetPower(address, self.eth.blockchain.CurrentBlock().Number()).Cmp(minPower) < 0 {
				fmt.Println(logTime, "Insufficient power for ping transaction.", address.Hex(), self.eth.blockchain.CurrentBlock().Number().String(), stateDB.GetPower(address, self.eth.blockchain.CurrentBlock().Number()).String())
				break
			}
			tx := types.NewTransaction(
				self.eth.txPool.State().GetNonce(address),
				params.MasterndeContractAddress,
				big.NewInt(0),
				gas,
				gasPrice,
				nil,
			)
			signed, err := types.SignTx(tx, types.NewEIP155Signer(self.eth.blockchain.Config().ChainID), self.PrivateKey)
			if err != nil {
				fmt.Println(logTime, "SignTx error:", err)
				break
			}

			if err := self.eth.txPool.AddLocal(signed); err != nil {
				fmt.Println(logTime, "send ping to txpool error:", err)
				break
			}
			fmt.Println(logTime, "Send ping message!")
		}
	}
}

// SignHash calculates a ECDSA signature for the given hash. The produced
// signature is in the [R || S || V] format where V is 0 or 1.
func (self *MasternodeManager) SignHash(id string, hash []byte) ([]byte, error) {
	// Look up the key to sign with and abort if it cannot be found
	self.mu.RLock()
	defer self.mu.RUnlock()

	if id != self.ID {
		return nil, ErrUnknownMasternode
	}
	// Sign the hash using plain ECDSA operations
	return crypto.Sign(hash, self.PrivateKey)
}

func (self *MasternodeManager) checkSyncing() {
	events := self.mux.Subscribe(downloader.StartEvent{}, downloader.DoneEvent{}, downloader.FailedEvent{})
	for ev := range events.Chan() {
		switch ev.Data.(type) {
		case downloader.StartEvent:
			atomic.StoreInt32(&self.syncing, 1)
		case downloader.DoneEvent, downloader.FailedEvent:
			atomic.StoreInt32(&self.syncing, 0)
		}
	}
}

func (self *MasternodeManager) MasternodeList(number *big.Int) ([]string, error) {
	return masternode.GetIdsByBlockNumber(self.contract, number)
}

func (self *MasternodeManager) GetGovernanceContractAddress(number *big.Int) (common.Address, error) {
	return masternode.GetGovernanceAddress(self.contract, number)
}