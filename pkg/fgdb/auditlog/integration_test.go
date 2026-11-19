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
	setConfig := func(t *testing.T, config string) {
		t.Helper()
		expected, err := auditlogging.Parse(config)
		if err != nil {
			t.Fatal(err)
		}
		setup.Exec(t, "SET CLUSTER SETTING fgdb.sql.log.user_audit = $1", config)
		// The setting value can become visible before its OnChange callback
		// publishes the parsed configuration used by statements.
		testutils.SucceedsSoon(t, func() error {
			execCfg.AuditConfig.RLock()
			defer execCfg.AuditConfig.RUnlock()
			if !reflect.DeepEqual(execCfg.AuditConfig.Config, expected) {
				return fmt.Errorf("parsed audit configuration has not been published")
			}
			return nil
		})
	}
	setEnabled := func(t *testing.T, value bool) {
		t.Helper()
		setup.Exec(t, "SET CLUSTER SETTING fgdb.role_audit_log.enabled = $1", value)
		testutils.SucceedsSoon(t, func() error {
			if got := auditlogging.UserAuditEnabled(st, uuid.Nil); got != value {
				return fmt.Errorf("audit enabled=%t, want %t", got, value)
			}
			return nil
		})
	}
	setConfig(t, "test_role ALL\nALL NONE")

	spy := logtestutils.NewStructuredLogSpy(t,
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
	t.Cleanup(log.InterceptWith(ctx, spy))

	newSession := func(t *testing.T) *sqlutils.SQLRunner {
		t.Helper()
		db := s.ApplicationLayer().SQLConn(t, serverutils.User("testuser"))
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
	check := func(t *testing.T, session *sqlutils.SQLRunner, wantRole string) {
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

	session := newSession(t)
	t.Run("gate off", func(t *testing.T) { check(t, session, "") })
	setEnabled(t, true)
	t.Run("enabled role event", func(t *testing.T) { check(t, session, "test_role") })
	t.Run("live config NONE", func(t *testing.T) {
		setConfig(t, "test_role NONE\nALL ALL")
		check(t, session, "")
	})
	t.Run("live config ALL", func(t *testing.T) {
		setConfig(t, "ALL ALL\ntest_role NONE")
		check(t, session, "all")
	})
	t.Run("enabled empty", func(t *testing.T) { setConfig(t, ""); check(t, session, "") })
	t.Run("reduced config caches per session", func(t *testing.T) {
		setConfig(t, "test_role ALL\nALL NONE")
		setup.Exec(t, "SET CLUSTER SETTING fgdb.sql.log.user_audit.reduced_config.enabled = true")
		testutils.SucceedsSoon(t, func() error {
			if !auditlogging.UserAuditReducedConfigEnabled(&st.SV) {
				return fmt.Errorf("reduced audit configuration is not enabled")
			}
			return nil
		})
		cached := newSession(t)
		check(t, cached, "test_role")
		setConfig(t, "ALL NONE")
		check(t, cached, "test_role")
		setConfig(t, "")
		check(t, cached, "")
		setConfig(t, "ALL NONE")
		excluded := newSession(t)
		check(t, excluded, "")
		setConfig(t, "ALL ALL")
		check(t, excluded, "")
		check(t, newSession(t), "all")
		setEnabled(t, false)
		check(t, cached, "")
	})
}
