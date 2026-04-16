// Netsy <https://netsy.dev>
// Copyright 2026 Nadrama Pty Ltd
// SPDX-License-Identifier: Apache-2.0

package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"github.com/nadrama-com/netsy/internal/config"
)

type Role string

const (
	RolePeer   Role = "peer"
	RoleClient Role = "client"
)

// ValidateLocalNodeCertificates checks the configured local peer certificates against node identity and advertised server addresses.
func ValidateLocalNodeCertificates(c *config.Config, tlsFiles *config.TLSFiles) error {
	if tlsFiles == nil {
		return fmt.Errorf("tls files missing")
	}
	if err := validatePeerSubject(tlsFiles.ServerCert.Leaf, c.NodeID, c.ClusterID); err != nil {
		return fmt.Errorf("invalid server certificate subject: %w", err)
	}
	if err := validatePeerSubject(tlsFiles.ClientCert.Leaf, c.NodeID, c.ClusterID); err != nil {
		return fmt.Errorf("invalid client certificate subject: %w", err)
	}
	if err := validateSANs(tlsFiles.ServerCert.Leaf, c.AdvertiseClient, c.AdvertisePeer, c.AdvertiseElection); err != nil {
		return fmt.Errorf("invalid server certificate SANs: %w", err)
	}
	return nil
}

// NewServerTLSConfig builds a server-side TLS configuration that only accepts client certificates with the given role.
func NewServerTLSConfig(c *config.Config, tlsFiles *config.TLSFiles, allowedRole Role) (*tls.Config, error) {
	if tlsFiles == nil {
		return nil, fmt.Errorf("tls files missing")
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{tls.TLS_AES_256_GCM_SHA384},
		Certificates: []tls.Certificate{
			*tlsFiles.ServerCert,
		},
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  tlsFiles.ClientCA,
		VerifyConnection: func(state tls.ConnectionState) error {
			return verifyConnection(state, c.ClusterID, allowedRole)
		},
	}, nil
}

// verifyConnection checks the peer certificate's role and cluster identity on an established TLS connection.
func verifyConnection(state tls.ConnectionState, clusterID string, allowedRole Role) error {
	if len(state.VerifiedChains) == 0 {
		return fmt.Errorf("peer certificate was not verified")
	}
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("peer certificate missing")
	}

	cert := state.PeerCertificates[0]
	role, err := certRole(cert)
	if err != nil {
		return err
	}
	if role != allowedRole {
		return fmt.Errorf("certificate role %q is not allowed", role)
	}
	if err := checkOrganization(cert, clusterID); err != nil {
		return err
	}

	switch role {
	case RolePeer:
		if err := config.ValidateIdentifier(cert.Subject.CommonName, "peer certificate common name"); err != nil {
			return err
		}
	case RoleClient:
		if cert.Subject.CommonName == "" {
			return fmt.Errorf("client certificate common name is required")
		}
	}

	return nil
}

// NewClientTLSConfig builds a client-side TLS configuration for outbound
// peer gRPC connections. The local node presents its client certificate
// and verifies the remote server against the cluster CA.
func NewClientTLSConfig(tlsFiles *config.TLSFiles) (*tls.Config, error) {
	if tlsFiles == nil {
		return nil, fmt.Errorf("tls files missing")
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{tls.TLS_AES_256_GCM_SHA384},
		Certificates: []tls.Certificate{
			*tlsFiles.ClientCert,
		},
		RootCAs: tlsFiles.ServerCA,
	}, nil
}

// validatePeerSubject checks that a local node certificate has the peer role and matches the expected node and cluster identity.
func validatePeerSubject(cert *x509.Certificate, nodeID, clusterID string) error {
	if cert == nil {
		return fmt.Errorf("certificate missing")
	}
	if err := checkOrganization(cert, clusterID); err != nil {
		return err
	}
	role, err := certRole(cert)
	if err != nil {
		return err
	}
	if role != RolePeer {
		return fmt.Errorf("certificate role must be %q, got %q", RolePeer, role)
	}
	if cert.Subject.CommonName != nodeID {
		return fmt.Errorf("certificate common name must match node_id %q, got %q", nodeID, cert.Subject.CommonName)
	}
	return nil
}

// validateSANs checks that the server certificate's SANs cover every configured advertise address.
func validateSANs(cert *x509.Certificate, advertiseAddrs ...string) error {
	if cert == nil {
		return fmt.Errorf("certificate missing")
	}

	seenHosts := make(map[string]struct{})
	for _, addr := range advertiseAddrs {
		if addr == "" {
			continue
		}
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("invalid advertise address %q: %w", addr, err)
		}
		if host == "" {
			return fmt.Errorf("advertise address %q must include a host", addr)
		}
		if _, ok := seenHosts[host]; ok {
			continue
		}
		seenHosts[host] = struct{}{}
		if err := cert.VerifyHostname(host); err != nil {
			return fmt.Errorf("server certificate SANs do not cover advertise address %q: %w", addr, err)
		}
	}

	return nil
}

// checkOrganization requires the certificate's Organization field to match the cluster ID.
func checkOrganization(cert *x509.Certificate, clusterID string) error {
	if len(cert.Subject.Organization) != 1 {
		return fmt.Errorf("certificate must contain exactly one organization")
	}
	if cert.Subject.Organization[0] != clusterID {
		return fmt.Errorf("certificate organization must match cluster_id %q, got %q", clusterID, cert.Subject.Organization[0])
	}
	return nil
}

// certRole extracts and validates the role from the certificate's OrganizationalUnit field.
func certRole(cert *x509.Certificate) (Role, error) {
	if len(cert.Subject.OrganizationalUnit) != 1 {
		return "", fmt.Errorf("certificate must contain exactly one organizational unit")
	}
	role := Role(cert.Subject.OrganizationalUnit[0])
	switch role {
	case RolePeer, RoleClient:
		return role, nil
	default:
		return "", fmt.Errorf("certificate organizational unit must be %q or %q, got %q", RolePeer, RoleClient, cert.Subject.OrganizationalUnit[0])
	}
}

// PeerNodeID extracts the peer's node ID (CN field) from the gRPC context's TLS peer certificates.
func PeerNodeID(ctx context.Context) (string, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return "", fmt.Errorf("no peer info in context")
	}
	if p.AuthInfo == nil {
		return "", fmt.Errorf("no auth info in peer")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", fmt.Errorf("peer auth info is not TLS")
	}
	if len(tlsInfo.State.PeerCertificates) == 0 {
		return "", fmt.Errorf("no peer certificates")
	}
	cn := tlsInfo.State.PeerCertificates[0].Subject.CommonName
	if cn == "" {
		return "", fmt.Errorf("peer certificate common name is empty")
	}
	return cn, nil
}
