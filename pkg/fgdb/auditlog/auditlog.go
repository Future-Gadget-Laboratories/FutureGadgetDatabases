// Copyright 2026 Future Gadget Laboratories
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package auditlog installs FGDb's gated role-based SQL audit settings.
// See docs/fgdb/features/role-audit-log.md.
package auditlog

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/fgdb/gate"
	"github.com/cockroachdb/cockroach/pkg/settings"
	"github.com/cockroachdb/cockroach/pkg/settings/cluster"
	"github.com/cockroachdb/cockroach/pkg/sql/auditlogging"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/cockroach/pkg/util/uuid"
)

// See docs/fgdb/features/role-audit-log.md.
var enabled = func() *settings.BoolSetting {
	name, err := gate.SettingName("role_audit_log")
	if err != nil {
		panic(err)
	}
	return settings.RegisterBoolSetting(settings.ApplicationLevel, settings.InternalKey(name),
		"enable FGDb role-based SQL audit logging", false, settings.WithPublic)
}()

// See docs/fgdb/features/role-audit-log.md.
var userAudit = settings.RegisterStringSetting(
	settings.ApplicationLevel, "fgdb.sql.log.user_audit",
	"ordered user or role and ALL/NONE statement filters for FGDb SQL audit logging", "",
	settings.WithPublic,
	settings.WithValidateString(func(_ *settings.Values, value string) error {
		_, err := auditlogging.Parse(value)
		return err
	}),
)

// See docs/fgdb/features/role-audit-log.md.
var reducedConfig = settings.RegisterBoolSetting(
	settings.ApplicationLevel, "fgdb.sql.log.user_audit.reduced_config.enabled",
	"cache the matching FGDb audit rule at the first audited SQL event per session", false,
	settings.WithPublic,
)

func init() {
	auditlogging.UserAuditEnabled = func(st *cluster.Settings, _ uuid.UUID) bool {
		return enabled.Get(&st.SV) && userAudit.Get(&st.SV) != ""
	}
	auditlogging.UserAuditReducedConfigEnabled = func(sv *settings.Values) bool {
		return reducedConfig.Get(sv)
	}
	auditlogging.ConfigureRoleBasedAuditClusterSettings = configure
}

func configure(
	ctx context.Context, acl *auditlogging.AuditConfigLock, _ *cluster.Settings, sv *settings.Values,
) {
	reload := func(ctx context.Context) {
		// Read the setting under the same lock as publication so concurrent
		// callbacks cannot publish an older snapshot after a newer one.
		acl.Lock()
		err := applyConfig(acl, userAudit.Get(sv))
		acl.Unlock()
		if err != nil {
			// Parser errors can contain user-supplied role names. Do not log the
			// error or the raw setting; report the failure without exposing either.
			log.Warningf(ctx, "invalid fgdb.sql.log.user_audit; retaining previous audit configuration")
		}
	}
	// Register before the initial read to avoid missing a concurrent change.
	userAudit.SetOnChange(sv, reload)
	reload(ctx)
}

// applyConfig replaces only successfully parsed configurations. The caller must
// hold acl's write lock, including while obtaining the current setting value.
func applyConfig(acl *auditlogging.AuditConfigLock, value string) error {
	config, err := auditlogging.Parse(value)
	if err != nil {
		return err
	}
	acl.Config = config
	return nil
}
