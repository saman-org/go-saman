// Copyright 2017 The go-saman Authors
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

package core

// Constants containing the genesis allocation of built-in genesis blocks.
// Their content is an RLP-encoded list of (address, balance) tuples.
// Use mkalloc.go to create/update them.

// nolint: misspell

const testnetAllocData = "\xc0"
const rinkebyAllocData = "\xc0"
const mainnetAllocData = "\xe2\xe1\x94.\xe01\x8a\xfb\x9d\xb2\xe9\xbc\xda\xf6\xe2h\xa7u\f\x14H\xf7]\x8bR\xb7\xd2\xdc\xc8\f\xd2\xe4\x00\x00\x00"