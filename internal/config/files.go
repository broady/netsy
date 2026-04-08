// Copyright 2025 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

type TLSFiles struct {
	ServerCert *tls.Certificate
	ServerCA   *x509.CertPool
	ClientCert *tls.Certificate
	ClientCA   *x509.CertPool
}

func LoadTLSFiles(c *Config) (*TLSFiles, error) {
	// Load server and client certificates and keys
	serverCert, err := tls.LoadX509KeyPair(c.TLSServerCert, c.TLSServerKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load server cert %s and key %s: %w", c.TLSServerCert, c.TLSServerKey, err)
	}
	clientCert, err := tls.LoadX509KeyPair(c.TLSClientCert, c.TLSClientKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load client cert %s and key %s: %w", c.TLSClientCert, c.TLSClientKey, err)
	}

	// Load single CA pool used for both server and client
	caPool := x509.NewCertPool()
	caPem, err := os.ReadFile(c.TLSCACert)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file: %w", err)
	}
	if !caPool.AppendCertsFromPEM(caPem) {
		return nil, fmt.Errorf("failed to append CA cert to pool")
	}

	return &TLSFiles{
		ServerCA:   caPool,
		ServerCert: &serverCert,
		ClientCA:   caPool,
		ClientCert: &clientCert,
	}, nil
}
