// Copyright 2026 Future Gadget Laboratories.
//
// Licensed under the Apache License, Version 2.0. See licenses/APL.txt.

package scenario

// Claim ids match fgdb/test/claims.yaml. The registry test fails if they drift.
const (
	ClaimRepl        = "REPL-001"
	ClaimKill        = "FT-001"
	ClaimPause       = "FT-002"
	ClaimKillTwo     = "FT-003"
	ClaimClients     = "CLIENT-001"
	ClaimBank        = "CONS-001"
	ClaimPorcupine   = "CONS-002"
	ClaimConsistency = "CONS-003"
	ClaimSQL         = "SQL-001"
	ClaimKV          = "KV-001"
	ClaimBackup      = "BAK-001"
	ClaimUpgrade     = "UP-001"
	ClaimPartition   = "FT-010"
	ClaimDisk        = "FT-011"
)

const (
	stepUpgrade     = "rolling-upgrade"
	stepKill        = "node-kill"
	stepPause       = "node-pause"
	stepKillTwo     = "kill-two"
	stepPorcupine   = "porcupine"
	stepConsistency = "consistency"
	stepBackup      = "backup-roundtrip"
)

// ClusterClaims are the claims the 3-node scenario itself records.
func ClusterClaims() []string {
	return []string{
		ClaimRepl,
		ClaimKill,
		ClaimPause,
		ClaimKillTwo,
		ClaimClients,
		ClaimBank,
		ClaimPorcupine,
		ClaimConsistency,
		ClaimBackup,
		ClaimUpgrade,
		ClaimPartition,
		ClaimDisk,
	}
}
