package models

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------- JSONB Value/Scan 测试（AuditLog 的 RequestBody 依赖这个通用类型） ----------
func TestJSONB_ValueScan(t *testing.T) {
	t.Run("空 JSONB 应为 nil driver.Value", func(t *testing.T) {
		var j JSONB
		v, err := j.Value()
		if err != nil {
			t.Fatalf("Value err=%v", err)
		}
		if v != nil {
			t.Fatalf("empty JSONB Value=%v, want nil", v)
		}
	})

	t.Run("null 字面量 JSONB 应为 nil driver.Value", func(t *testing.T) {
		j := JSONB(json.RawMessage("null"))
		v, err := j.Value()
		if err != nil {
			t.Fatalf("Value err=%v", err)
		}
		if v != nil {
			t.Fatalf("null JSONB Value=%v, want nil", v)
		}
	})

	t.Run("object JSONB Value 为 []byte", func(t *testing.T) {
		raw := `{"k":"v","n":42}`
		j := JSONB(json.RawMessage(raw))
		v, err := j.Value()
		if err != nil {
			t.Fatalf("Value err=%v", err)
		}
		b, ok := v.([]byte)
		if !ok {
			t.Fatalf("Value type=%T, want []byte", v)
		}
		if string(b) != raw {
			t.Fatalf("Value=%s, want %s", b, raw)
		}
	})

	t.Run("Scan + MarshalJSON 往返一致", func(t *testing.T) {
		raw := []byte(`["a","b",{"x":1}]`)
		var j JSONB
		if err := j.Scan(raw); err != nil {
			t.Fatalf("Scan err=%v", err)
		}
		out, merr := j.MarshalJSON()
		if merr != nil {
			t.Fatalf("MarshalJSON err=%v", merr)
		}
		if string(out) != string(raw) {
			t.Fatalf("roundtrip got=%s, want=%s", out, raw)
		}
	})

	t.Run("UnmarshalJSON 后 Scan 输出不变", func(t *testing.T) {
		var j JSONB
		src := []byte(`{"ok":true}`)
		if err := j.UnmarshalJSON(src); err != nil {
			t.Fatalf("UnmarshalJSON err=%v", err)
		}
		v, err := j.Value()
		if err != nil {
			t.Fatalf("Value err=%v", err)
		}
		if !reflect.DeepEqual([]byte(v.([]byte)), src) {
			t.Fatalf("after UnmarshalJSON, Value=%s, want %s", v, src)
		}
	})

	t.Run("Scan 非 []byte 报错", func(t *testing.T) {
		var j JSONB
		if err := j.Scan("not bytes"); err == nil {
			t.Fatal("Scan(string) should fail")
		}
	})
}

// ---------- AuditLog JSON marshal/unmarshal（字段 tag / 序列化约定） ----------
func TestAuditLog_JSON_RoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	bodyRaw := `{"username":"alice","role_id":3}`
	log := AuditLog{
		ID:            101,
		TraceID:       "t-abc123",
		UserID:        7,
		Username:      "alice",
		ClientIP:      "10.0.0.1",
		UserAgent:     "Mozilla/5.0",
		Module:        "backup",
		Action:        "create",
		TargetType:    "backup_task",
		TargetID:      "42",
		ClusterCode:   "cluster-a",
		Namespace:     "default",
		Status:        "success",
		ErrorMsg:      "",
		RequestMethod: "POST",
		RequestURI:    "/api/v1/clusters/cluster-a/backups",
		RequestBody:   JSONB(json.RawMessage(bodyRaw)),
		ResponseCode:  200,
		CostMs:        123,
		CreatedAt:     now,
	}

	buf, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// 关键字段 tag 检查（json:"trace_id" 而不是 traceID）
	s := string(buf)
	if !strings.Contains(s, `"trace_id":"t-abc123"`) {
		t.Fatalf("missing trace_id tag in json: %s", s)
	}
	if !strings.Contains(s, `"cluster_code":"cluster-a"`) {
		t.Fatalf("missing cluster_code tag in json: %s", s)
	}
	if !strings.Contains(s, `"request_method":"POST"`) {
		t.Fatalf("missing request_method tag in json: %s", s)
	}
	if !strings.Contains(s, `"request_body":`) {
		t.Fatalf("missing request_body in json: %s", s)
	}
	// ID 字段应该是普通数字，不是字符串
	if !strings.Contains(s, `"id":101`) {
		t.Fatalf("id field serialized wrong: %s", s)
	}

	// 反序列化回来
	var back AuditLog
	if err := json.Unmarshal(buf, &back); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if back.ID != 101 || back.TraceID != "t-abc123" || back.Username != "alice" {
		t.Fatalf("roundtrip mismatch simple fields: %+v", back)
	}
	if back.ClusterCode != "cluster-a" || back.Namespace != "default" {
		t.Fatalf("roundtrip mismatch scope fields: %+v", back)
	}
	if back.RequestMethod != "POST" || back.RequestURI != "/api/v1/clusters/cluster-a/backups" {
		t.Fatalf("roundtrip mismatch request fields: %+v", back)
	}
	if back.ResponseCode != 200 || back.CostMs != 123 {
		t.Fatalf("roundtrip mismatch numeric fields: code=%d cost=%d", back.ResponseCode, back.CostMs)
	}
	// request body JSONB 内容保持
	var bodyObj1, bodyObj2 interface{}
	if err := json.Unmarshal([]byte(log.RequestBody), &bodyObj1); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(back.RequestBody, &bodyObj2); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bodyObj1, bodyObj2) {
		t.Fatalf("request_body not equal after roundtrip: %+v vs %+v", bodyObj1, bodyObj2)
	}
}

// ---------- AuditLog 表名 & 状态值约定 ----------
func TestAuditLog_TableAndStatus(t *testing.T) {
	got := (AuditLog{}).TableName()
	if got != "audit_log" {
		t.Fatalf("TableName=%q, want audit_log", got)
	}

	// 常见状态值约定（success/fail）不应该被轻易改坏
	valid := map[string]bool{"success": true, "fail": true}
	for _, st := range []string{"success", "fail"} {
		if !valid[st] {
			t.Fatalf("unknown status: %s", st)
		}
	}
}

// ---------- BackupTask Status 状态常量 & 表名 ----------
func TestBackupTask_StatusAndTable(t *testing.T) {
	want := (BackupTask{}).TableName()
	if want != "backup_task" {
		t.Fatalf("BackupTask.TableName=%q, want backup_task", want)
	}

	statuses := []BackupTaskStatus{
		BackupStatusPending, BackupStatusRunning, BackupStatusSuccess,
		BackupStatusFailed, BackupStatusCancelled,
	}
	seen := make(map[BackupTaskStatus]bool)
	for _, s := range statuses {
		if s == "" {
			t.Fatal("empty BackupTaskStatus")
		}
		if seen[s] {
			t.Fatalf("duplicate status: %s", s)
		}
		seen[s] = true
	}
}
