// Copyright 2026 Future Gadget Laboratories
// Use of this source code is governed by the Apache License, Version 2.0
// that can be found in the LICENSE file.

package auditlog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/base"
	"github.com/cockroachdb/cockroach/pkg/security/securityassets"
	"github.com/cockroachdb/cockroach/pkg/security/securitytest"
	"github.com/cockroachdb/cockroach/pkg/server"
	"github.com/cockroachdb/cockroach/pkg/settings/cluster"
	"github.com/cockroachdb/cockroach/pkg/sql"
	"github.com/cockroachdb/cockroach/pkg/sql/auditlogging"
	"github.com/cockroachdb/cockroach/pkg/testutils"
	"github.com/cockroachdb/cockroach/pkg/testutils/serverutils"
	"github.com/cockroachdb/cockroach/pkg/testutils/sqlutils"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/log"
	"github.com/cockroachdb/cockroach/pkg/util/log/eventpb"
	"github.com/cockroachdb/cockroach/pkg/util/log/logpb"
	"github.com/cockroachdb/cockroach/pkg/util/log/logtestutils"
	"github.com/cockroachdb/cockroach/pkg/util/uuid"
	"github.com/cockroachdb/redact"
)

func TestMain(m *testing.M) {
	securityassets.SetLoader(securitytest.EmbeddedAssets)
	serverutils.InitTestServerFactory(server.TestServerFactory)
	os.Exit(m.Run())
}

func TestRoleAuditEvents(t *testing.T) {
	t.Cleanup(leaktest.AfterTest(t))
	scope := log.Scope(t)
	t.Cleanup(func() { scope.Close(t) })
	ctx := context.Background()
	s, sqlDB, _ := serverutils.StartServer(t, base.TestServerArgs{DefaultTestTenant: base.SharedTestTenantAlwaysEnabled})
	t.Cleanup(func() { s.Stopper().Stop(ctx) })
	setup := sqlutils.MakeSQLRunner(sqlDB)
	setup.Exec(t, "CREATE ROLE test_role")
	setup.Exec(t, "CREATE USER testuser")
	setup.Exec(t, "GRANT test_role TO testuser")
	execCfg := s.ApplicationLayer().ExecutorConfig().(sql.ExecutorConfig)
	st := s.ApplicationLayer().ClusterSettings()
	setAuditConfig(t, setup, execCfg.AuditConfig, "test_role ALL\nALL NONE")

	spy := newRoleAuditLogSpy(t)
	t.Cleanup(log.InterceptWith(ctx, spy))

	session := newAuditSession(t, ctx, s.ApplicationLayer())
	t.Run("gate off", func(t *testing.T) { checkRoleAuditEvents(t, session, spy, "") })
	setAuditEnabled(t, setup, st, true)
	t.Run("enabled role event", func(t *testing.T) { checkRoleAuditEvents(t, session, spy, "test_role") })
	t.Run("live config NONE", func(t *testing.T) {
		setAuditConfig(t, setup, execCfg.AuditConfig, "test_role NONE\nALL ALL")
		checkRoleAuditEvents(t, session, spy, "")
	})
	t.Run("live config ALL", func(t *testing.T) {
		setAuditConfig(t, setup, execCfg.AuditConfig, "ALL ALL\ntest_role NONE")
		checkRoleAuditEvents(t, session, spy, "all")
	})
	t.Run("enabled empty", func(t *testing.T) {
		setAuditConfig(t, setup, execCfg.AuditConfig, "")
		checkRoleAuditEvents(t, session, spy, "")
	})
	t.Run("reduced config caches per session", func(t *testing.T) {
		checkReducedAuditConfigCachesPerSession(t, ctx, s.ApplicationLayer(), setup, spy)
	})
}

func setAuditConfig(
	t *testing.T, setup *sqlutils.SQLRunner, acl *auditlogging.AuditConfigLock, config string,
) {
	t.Helper()
	expected, err := auditlogging.Parse(config)
	if err != nil {
		t.Fatal(err)
	}
	setup.Exec(t, "SET CLUSTER SETTING fgdb.sql.log.user_audit = $1", config)
	// The setting value can become visible before its OnChange callback
	// publishes the parsed configuration used by statements.
	testutils.SucceedsSoon(t, func() error {
		acl.RLock()
		defer acl.RUnlock()
		if !reflect.DeepEqual(acl.Config, expected) {
			return fmt.Errorf("parsed audit configuration has not been published")
		}
		return nil
	})
}

func setAuditEnabled(
	t *testing.T, setup *sqlutils.SQLRunner, st *cluster.Settings, value bool,
) {
	t.Helper()
	setup.Exec(t, "SET CLUSTER SETTING fgdb.role_audit_log.enabled = $1", value)
	testutils.SucceedsSoon(t, func() error {
		if got := auditlogging.UserAuditEnabled(st, uuid.Nil); got != value {
			return fmt.Errorf("audit enabled=%t, want %t", got, value)
		}
		return nil
	})
}

func newRoleAuditLogSpy(t *testing.T) *logtestutils.StructuredLogSpy[eventpb.RoleBasedAuditEvent] {
	t.Helper()
	return logtestutils.NewStructuredLogSpy(t,
		[]logpb.Channel{logpb.Channel_SENSITIVE_ACCESS}, []string{"role_based_audit_event"},
		func(entry logpb.Entry) (eventpb.RoleBasedAuditEvent, error) {
			var event eventpb.RoleBasedAuditEvent
			err := json.Unmarshal([]byte(entry.Message[entry.StructuredStart:entry.StructuredEnd]), &event)
			return event, err
		},
		func(_ logpb.Entry, event eventpb.RoleBasedAuditEvent) bool {
			return redact.RedactableString(event.User).StripMarkers() == "testuser"
		},
	)
}

func newAuditSession(
	t *testing.T, ctx context.Context, app serverutils.ApplicationLayerInterface,
) *sqlutils.SQLRunner {
	t.Helper()
	db := app.SQLConn(t, serverutils.User("testuser"))
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return sqlutils.MakeSQLRunner(conn)
}

func checkRoleAuditEvents(
	t *testing.T, session *sqlutils.SQLRunner,
	spy *logtestutils.StructuredLogSpy[eventpb.RoleBasedAuditEvent], wantRole string,
) {
	t.Helper()
	log.FlushAllSync()
	before := spy.Count()
	session.CheckQueryResults(t, "SELECT 210", [][]string{{"210"}})
	log.FlushAllSync()
	events := spy.GetLogs(logpb.Channel_SENSITIVE_ACCESS)[before:]
	if wantRole == "" {
		if len(events) != 0 {
			t.Fatalf("unexpected audit events: %v", events)
		}
		return
	}
	if len(events) != 1 {
		t.Fatalf("got %d audit events, want 1", len(events))
	}
	if role := redact.RedactableString(events[0].Role).StripMarkers(); role != wantRole {
		t.Fatalf("role=%q, want %q", role, wantRole)
	}
	if events[0].Statement.StripMarkers() != "SELECT 210" {
		t.Fatalf("statement=%s", events[0].Statement)
	}
}

func checkReducedAuditConfigCachesPerSession(
	t *testing.T, ctx context.Context, app serverutils.ApplicationLayerInterface,
	setup *sqlutils.SQLRunner, spy *logtestutils.StructuredLogSpy[eventpb.RoleBasedAuditEvent],
) {
	t.Helper()
	execCfg := app.ExecutorConfig().(sql.ExecutorConfig)
	st := app.ClusterSettings()
	setAuditConfig(t, setup, execCfg.AuditConfig, "test_role ALL\nALL NONE")
	setup.Exec(t, "SET CLUSTER SETTING fgdb.sql.log.user_audit.reduced_config.enabled = true")
	testutils.SucceedsSoon(t, func() error {
		if !auditlogging.UserAuditReducedConfigEnabled(&st.SV) {
			return fmt.Errorf("reduced audit configuration is not enabled")
		}
		return nil
	})
	cached := newAuditSession(t, ctx, app)
	checkRoleAuditEvents(t, cached, spy, "test_role")
	setAuditConfig(t, setup, execCfg.AuditConfig, "ALL NONE")
	checkRoleAuditEvents(t, cached, spy, "test_role")
	setAuditConfig(t, setup, execCfg.AuditConfig, "")
	checkRoleAuditEvents(t, cached, spy, "")
	setAuditConfig(t, setup, execCfg.AuditConfig, "ALL NONE")
	excluded := newAuditSession(t, ctx, app)
	checkRoleAuditEvents(t, excluded, spy, "")
	setAuditConfig(t, setup, execCfg.AuditConfig, "ALL ALL")
	checkRoleAuditEvents(t, excluded, spy, "")
	checkRoleAuditEvents(t, newAuditSession(t, ctx, app), spy, "all")
	setAuditEnabled(t, setup, st, false)
	checkRoleAuditEvents(t, cached, spy, "")
}
