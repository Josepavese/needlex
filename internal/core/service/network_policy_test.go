package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/josepavese/needlex/internal/config"
	"github.com/josepavese/needlex/internal/platform"
	"github.com/josepavese/needlex/internal/rendering"
)

func TestServicePublicPolicyWrapsOnlyAcquisition(t *testing.T) {
	cfg := config.Defaults()
	cfg.Network.Policy = string(platform.NetworkPolicyPublic)
	technicalClient := &http.Client{}
	service, err := NewWithRenderer(cfg, technicalClient, t.TempDir(), rendering.NoopRenderer{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if service.acquirer.Egress == nil {
		t.Fatal("acquirer has no egress enforcement")
	}
	if service.acquirer.Client == technicalClient {
		t.Fatal("acquisition client was not wrapped by the policy")
	}
	if service.httpClient != technicalClient {
		t.Fatal("technical endpoint client was replaced by the policy")
	}
	_, err = service.Read(context.Background(), ReadRequest{URL: "http://127.0.0.1:9471/private"})
	if !platform.IsEgressDenied(err) {
		t.Fatalf("read error = %v, want egress denial", err)
	}
}
