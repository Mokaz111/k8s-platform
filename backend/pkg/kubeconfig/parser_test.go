package kubeconfig

import "testing"

func TestParseRejectsExec(t *testing.T) {
	raw := []byte(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1
  name: c
contexts:
- context:
    cluster: c
    user: u
  name: ctx
current-context: ctx
users:
- name: u
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: /bin/true
`)
	_, err := Parse(raw)
	if err == nil {
		t.Fatal("expected exec kubeconfig to be rejected")
	}
}

func TestParseRejectsAuthProvider(t *testing.T) {
	raw := []byte(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1
  name: c
contexts:
- context:
    cluster: c
    user: u
  name: ctx
current-context: ctx
users:
- name: u
  user:
    auth-provider:
      name: oidc
`)
	_, err := Parse(raw)
	if err == nil {
		t.Fatal("expected auth-provider kubeconfig to be rejected")
	}
}
