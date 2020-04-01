// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2019-2020 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package main_test

import (
	"os"

	"github.com/chrisccoulson/go-tpm2"
	"github.com/snapcore/secboot"
	. "gopkg.in/check.v1"

	main "github.com/snapcore/snapd/cmd/snap-bootstrap"
)

func (s *cmdSuite) TestGetCertificate(c *C) {
	tcti, err := os.Open("/dev/null")
	c.Assert(err, IsNil)
	tpm, err := tpm2.NewTPMContext(tcti)
	c.Assert(err, IsNil)
	mockTPM := &secboot.TPMConnection{TPMContext: tpm}
	restoreConnect := main.MockSecbootConnectToDefaultTPM(func() (*secboot.TPMConnection, error) {
		return mockTPM, nil
	})
	defer restoreConnect()

	n := 0
	restoreFetch := main.MockSecbootFetchAndSaveEKCertificateChain(func(tpm *secboot.TPMConnection, parentOnly bool, destPath string) error {
		c.Assert(tpm, Equals, mockTPM)
		c.Assert(parentOnly, Equals, false)
		c.Assert(destPath, Equals, "my/path")
		n++
		return nil
	})
	defer restoreFetch()

	rest, err := main.Parser().ParseArgs([]string{"get-certificate", "--ek", "my/path"})
	c.Assert(err, IsNil)
	c.Assert(rest, HasLen, 0)
	c.Assert(n, Equals, 1)
}
