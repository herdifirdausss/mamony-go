// Package handler contains non-routed Identity HTTP-boundary primitives.
// This package intentionally does not register or expose any HTTP endpoint.
package handler

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"github.com/herdifirdausss/mamony-go/internal/modules/identity/internal/usecase"
)

var ErrClientPeerUntrusted = errors.New("identity client address unavailable")

// ClientIPResolver is default-deny on forwarded headers: ONLY the direct TCP
// peer is trusted unless an explicitly enumerated private/loopback proxy
// replaces X-Forwarded-For with one canonical IP before passing the request.
// A proxy MUST strip all incoming forwarding headers at the network boundary.
type ClientIPResolver struct {
	proxies []netip.Prefix
}

func NewClientIPResolver(proxies []netip.Prefix) (*ClientIPResolver, error) {
	r := &ClientIPResolver{proxies: make([]netip.Prefix, 0, len(proxies))}
	if len(proxies) > 16 {
		return nil, ErrClientPeerUntrusted
	}
	for _, prefix := range proxies {
		if !prefix.IsValid() || prefix.Addr().Zone() != "" || prefix.Addr().Is4In6() ||
			!(prefix.Addr().IsPrivate() || prefix.Addr().IsLoopback()) ||
			(prefix.Addr().Is4() && prefix.Bits() < 24) ||
			(prefix.Addr().Is6() && prefix.Bits() < 64) {
			return nil, ErrClientPeerUntrusted
		}
		r.proxies = append(r.proxies, prefix.Masked())
	}
	return r, nil
}

func safeClientAddress(ip netip.Addr) bool {
	return ip.IsValid() && ip.Zone() == "" && !ip.IsUnspecified() && !ip.IsMulticast()
}

// Resolve never selects an untrusted hop from an attacker-controlled list.
// Trusted reverse proxies must supply exactly one sanitized XFF field. Every
// other proxy header is rejected at this boundary to avoid disagreement.
func (r *ClientIPResolver) Resolve(req *http.Request) (netip.Addr, error) {
	if r == nil || req == nil {
		return netip.Addr{}, ErrClientPeerUntrusted
	}
	peer, err := netip.ParseAddrPort(req.RemoteAddr)
	if err != nil || peer.Port() == 0 || !safeClientAddress(peer.Addr()) {
		return netip.Addr{}, ErrClientPeerUntrusted
	}
	remote := peer.Addr().Unmap()
	trusted := false
	for _, prefix := range r.proxies {
		if prefix.Contains(remote) {
			trusted = true
			break
		}
	}
	if !trusted {
		// X-Forwarded-For, Forwarded, X-Real-IP, etc. are untrusted input
		// when the immediate network peer is not a configured proxy.
		return remote, nil
	}
	values := req.Header.Values("X-Forwarded-For")
	if len(values) != 1 || values[0] == "" ||
		strings.ContainsAny(values[0], ",; \t\r\n") ||
		len(req.Header.Values("Forwarded")) != 0 ||
		len(req.Header.Values("X-Real-Ip")) != 0 ||
		len(req.Header.Values("True-Client-Ip")) != 0 {
		return netip.Addr{}, ErrClientPeerUntrusted
	}
	client, err := netip.ParseAddr(values[0])
	if err != nil || !safeClientAddress(client) || client.Is4In6() {
		return netip.Addr{}, ErrClientPeerUntrusted
	}
	return client, nil
}

type AnonymousAdmissionGate interface {
	Admit(context.Context, usecase.AnonymousAction, string, netip.Addr) (bool, error)
}

// AnonymousBoundary is intentionally NOT an http.Handler. Its caller must
// still implement and review method/body/Origin/CSRF, account lookup, durable
// work enqueue and timing normalization before exposing registration/resend.
type AnonymousBoundary struct {
	resolver *ClientIPResolver
	gate     AnonymousAdmissionGate
}

func NewAnonymousBoundary(resolver *ClientIPResolver, gate AnonymousAdmissionGate) (*AnonymousBoundary, error) {
	if resolver == nil || gate == nil {
		return nil, ErrClientPeerUntrusted
	}
	return &AnonymousBoundary{resolver: resolver, gate: gate}, nil
}

func (b *AnonymousBoundary) Admit(req *http.Request, action usecase.AnonymousAction, email string) (bool, error) {
	if b == nil || b.resolver == nil || b.gate == nil || req == nil {
		return false, ErrClientPeerUntrusted
	}
	addr, err := b.resolver.Resolve(req)
	if err != nil {
		return false, ErrClientPeerUntrusted
	}
	ok, err := b.gate.Admit(req.Context(), action, email, addr)
	if err != nil {
		return false, usecase.ErrAnonymousAdmissionUnavailable
	}
	return ok, nil
}

// WriteOpaqueAdmissionReceipt is the same regardless of known/unknown email,
// consumed/available budget or existing subject state. It deliberately says
// nothing about account creation or email dispatch. No endpoint calls it yet.
func WriteOpaqueAdmissionReceipt(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("{\"status\":\"accepted\"}\n"))
}
