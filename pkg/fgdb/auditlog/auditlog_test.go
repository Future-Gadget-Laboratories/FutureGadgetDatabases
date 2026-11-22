// Copyright 2026 Future Gadget Laboratories
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auditlog

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/security/username"
	"github.com/cockroachdb/cockroach/pkg/settings"
	"github.com/cockroachdb/cockroach/pkg/settings/cluster"
	"github.com/cockroachdb/cockroach/pkg/sql/auditlogging"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgerror"
	"github.com/cockroachdb/cockroach/pkg/util/uuid"
)

const (
	testRole         = "test_role"
	testUser         = "audit_user"
	allRole          = "all"
	auditAllConfig   = "ALL ALL"
	emptyAuditConfig = ""
)

const (
	userAuditByteLimit = 64 << 10
	userAuditRowLimit  = 1000
)

func TestSettingNamesAndDefaults(t *testing.T) {
	st := cluster.MakeTestingClusterSettings()
	for _, tc := range []struct {
		name  settings.SettingName
		value string
	}{
		{"fgdb.role_audit_log.enabled", "false"},
		{"fgdb.sql.log.user_audit", emptyAuditConfig},
		{"fgdb.sql.log.user_audit.reduced_config.enabled", "false"},
	} {
		t.Run(string(tc.name), func(t *testing.T) {
			setting, ok, _ := settings.LookupForLocalAccess(tc.name, true)
			if !ok {
				t.Fatalf("setting %s is not registered", tc.name)
			}
			if setting.EncodedDefault() != tc.value || setting.Encoded(&st.SV) != tc.value {
				t.Fatalf("default=%q current=%q, want %q", setting.EncodedDefault(), setting.Encoded(&st.SV), tc.value)
			}
			if setting.Class() != settings.ApplicationLevel {
				t.Fatalf("class=%v, want ApplicationLevel", setting.Class())
			}
			if setting.Visibility() != settings.Public {
				t.Fatalf("visibility=%v, want Public", setting.Visibility())
			}
		})
	}
}

func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		valid        bool
	}{
		{"empty", emptyAuditConfig, true},
		{"ordered rows", "test_role ALL\nALL NONE", true},
		{"missing filter", testRole, false},
		{"invalid filter", "test_role SELECT", false},
		{"extra field", "test_role ALL extra", false},
		{"duplicate role", "test_role ALL\ntest_role NONE", false},
		{"reserved role", "public ALL", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := cluster.MakeTestingClusterSettings()
			err := settings.NewUpdater(&st.SV).Set(context.Background(), userAudit.InternalKey(), settings.EncodedValue{Value: tc.config, Type: "s"})
			if (err == nil) != tc.valid {
				t.Fatalf("validation error=%v, valid=%t", err, tc.valid)
			}
		})
	}
}

func TestUserAuditConfigLimits(t *testing.T) {
	roleRow := testRole + " ALL"
	atRowLimit := makeUserAuditRows(userAuditRowLimit)
	atByteLimit := roleRow + strings.Repeat(" ", userAuditByteLimit-len(roleRow))
	for _, tc := range []struct {
		name, config, wantLimit string
	}{
		{"exact row limit with blank lines", "\n \t\r\n" + atRowLimit, ""},
		{"over row limit", makeUserAuditRows(userAuditRowLimit + 1), "1000 non-empty rows"},
		{"over row limit with duplicate role", atRowLimit + testRole + "_0 ALL", "1000 non-empty rows"},
		{"exact byte limit", atByteLimit, ""},
		{"over byte limit", atByteLimit + " ", "65536 bytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := cluster.MakeTestingClusterSettings()
			err := userAudit.Validate(&st.SV, tc.config)
			if tc.wantLimit != "" {
				assertUserAuditLimitError(t, err, tc.wantLimit)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func assertUserAuditLimitError(t *testing.T, err error, wantLimit string) {
	t.Helper()
	if err == nil {
		t.Fatal("oversized configuration was accepted")
	}
	if code := pgerror.GetPGCode(err); code != pgcode.ConfigurationLimitExceeded {
		t.Errorf("SQLSTATE = %s, want %s", code, pgcode.ConfigurationLimitExceeded)
	}
	if strings.Contains(err.Error(), testRole) {
		t.Error("limit error exposed the submitted role")
	}
	if !strings.Contains(err.Error(), wantLimit) {
		t.Errorf("limit error did not name the limit %q", wantLimit)
	}
}

func makeUserAuditRows(count int) string {
	var config strings.Builder
	for i := 0; i < count; i++ {
		config.WriteString(fmt.Sprintf("%s_%d ALL\n", testRole, i))
	}
	return config.String()
}

func TestHookSemantics(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		gate   bool
		config string
		want   bool
	}{
		{"disabled empty", false, emptyAuditConfig, false},
		{"disabled configured", false, auditAllConfig, false},
		{"enabled empty", true, emptyAuditConfig, false},
		{"enabled configured", true, auditAllConfig, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := cluster.MakeTestingClusterSettings()
			enabled.Override(ctx, &st.SV, tc.gate)
			userAudit.Override(ctx, &st.SV, tc.config)
			if got := auditlogging.UserAuditEnabled(st, uuid.Nil); got != tc.want {
				t.Fatalf("enabled=%t, want %t", got, tc.want)
			}
			for _, reduced := range []bool{false, true} {
				reducedConfig.Override(ctx, &st.SV, reduced)
				if got := auditlogging.UserAuditReducedConfigEnabled(&st.SV); got != reduced {
					t.Fatalf("reduced=%t, want %t", got, reduced)
				}
			}
		})
	}
}

func TestConfigureAndOnChange(t *testing.T) {
	ctx := context.Background()
	st := cluster.MakeTestingClusterSettings()
	userAudit.Override(ctx, &st.SV, "test_role ALL\nALL NONE")
	acl := &auditlogging.AuditConfigLock{Config: auditlogging.EmptyAuditConfig()}
	auditlogging.ConfigureRoleBasedAuditClusterSettings(ctx, acl, st, &st.SV)
	role := username.MakeSQLUsernameFromPreNormalizedString(testRole)
	user := username.MakeSQLUsernameFromPreNormalizedString(testUser)
	roles := map[username.SQLUsername]bool{role: false}
	assertMatch(t, acl, roles, user, testRole, true)
	userAudit.Override(ctx, &st.SV, "ALL NONE")
	assertMatch(t, acl, roles, user, allRole, false)
	userAudit.Override(ctx, &st.SV, emptyAuditConfig)
	if got := acl.GetMatchingAuditSetting(roles, user); got != nil {
		t.Fatalf("empty config matched %v", got)
	}
}

func TestBadRuntimeConfigKeepsPrevious(t *testing.T) {
	acl := &auditlogging.AuditConfigLock{Config: auditlogging.EmptyAuditConfig()}
	// Exercise the runtime application boundary directly: settings validation
	// normally prevents an invalid value from reaching the change callback.
	acl.Lock()
	if err := applyConfig(acl, auditAllConfig); err != nil {
		acl.Unlock()
		t.Fatal(err)
	}
	previous := acl.Config
	err := applyConfig(acl, "ALL invalid")
	retained := acl.Config == previous
	acl.Unlock()
	if err == nil {
		t.Fatal("invalid runtime config succeeded")
	}
	if !retained {
		t.Fatal("invalid runtime config replaced previous configuration")
	}
	assertMatch(t, acl, nil, username.RootUserName(), allRole, true)
}

func TestOversizedRuntimeConfigKeepsPrevious(t *testing.T) {
	acl := &auditlogging.AuditConfigLock{Config: auditlogging.EmptyAuditConfig()}
	acl.Lock()
	defer acl.Unlock()
	if err := applyConfig(acl, auditAllConfig); err != nil {
		t.Fatal(err)
	}
	previous := acl.Config
	oversized := auditAllConfig + strings.Repeat(" ", userAuditByteLimit)
	err := applyConfig(acl, oversized)
	if code := pgerror.GetPGCode(err); code != pgcode.ConfigurationLimitExceeded {
		t.Errorf("SQLSTATE = %s, want %s", code, pgcode.ConfigurationLimitExceeded)
	}
	if acl.Config != previous {
		t.Error("oversized runtime config replaced previous configuration")
	}
}

func TestMatchingRules(t *testing.T) {
	user := username.MakeSQLUsernameFromPreNormalizedString(testUser)
	role := username.MakeSQLUsernameFromPreNormalizedString(testRole)
	for _, tc := range []struct {
		name, config, role string
		include            bool
	}{
		{"first match wins", "test_role NONE\naudit_user ALL\nALL ALL", testRole, false},
		{"username first", "audit_user ALL\ntest_role NONE", testUser, true},
		{"ALL role row", "ALL ALL\ntest_role NONE", allRole, true},
		{"NONE filter", "ALL NONE", allRole, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := cluster.MakeTestingClusterSettings()
			userAudit.Override(context.Background(), &st.SV, tc.config)
			acl := &auditlogging.AuditConfigLock{Config: auditlogging.EmptyAuditConfig()}
			auditlogging.ConfigureRoleBasedAuditClusterSettings(context.Background(), acl, st, &st.SV)
			assertMatch(t, acl, map[username.SQLUsername]bool{role: false}, user, tc.role, tc.include)
		})
	}
}

func assertMatch(t *testing.T, acl *auditlogging.AuditConfigLock, roles map[username.SQLUsername]bool, user username.SQLUsername, role string, include bool) {
	t.Helper()
	got := acl.GetMatchingAuditSetting(roles, user)
	if got == nil {
		t.Fatal("no matching audit setting")
	}
	if got.Role.Normalized() != role || got.IncludeStatements != include {
		t.Fatalf("matched role=%s include=%t, want role=%s include=%t", got.Role.Normalized(), got.IncludeStatements, role, include)
	}
}
