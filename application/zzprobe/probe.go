package zzprobe

import "github.com/Capsule7446/healix-core/domain/fingerprint"

// A: name says clone, signature is by pointer.
func cloneFingerprint(src *fingerprint.Fingerprint) *fingerprint.Fingerprint {
	out := *src
	out.Path = append([]string(nil), src.Path...)
	return &out
}

var _ = cloneFingerprint
