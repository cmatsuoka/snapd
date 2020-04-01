// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2019 Canonical Ltd
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

package main

import (
	"fmt"

	"github.com/jessevdk/go-flags"
	"github.com/snapcore/secboot"

	"github.com/snapcore/snapd/logger"
)

func init() {
	const (
		short = "Obtain certificates for TPM"
		long  = "Download and save certificates for the TPM device"
	)

	addCommandBuilder(func(parser *flags.Parser) {
		if _, err := parser.AddCommand("get-certificate", short, long, &cmdGetCertificate{}); err != nil {
			panic(err)
		}
	})
}

type cmdGetCertificate struct {
	EKCertsPath string `long:"ek" value-name:"filename" description:"Where the EK certificates will be stored"`
}

func (c *cmdGetCertificate) Execute(args []string) error {
	if c.EKCertsPath != "" {
		return getEKCertificate(c.EKCertsPath)
	}
	logger.Noticef("no certificates to download")
	return nil
}

var (
	secbootConnectToDefaultTPM            = secboot.ConnectToDefaultTPM
	secbootFetchAndSaveEKCertificateChain = secboot.FetchAndSaveEKCertificateChain
)

func getEKCertificate(destPath string) error {
	tpm, err := secbootConnectToDefaultTPM()
	if err != nil {
		return fmt.Errorf("cannot connect to TPM: %v", err)
	}
	defer tpm.Close()

	if err := secbootFetchAndSaveEKCertificateChain(tpm, false, destPath); err != nil {
		return fmt.Errorf("cannot fetch and save certificate chain: %v", err)
	}

	logger.Noticef("downloaded TPM EK certificates to %s", destPath)

	return nil
}
