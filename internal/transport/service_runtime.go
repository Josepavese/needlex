package transport

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/josepavese/needlex/internal/config"
	coreservice "github.com/josepavese/needlex/internal/core/service"
	"github.com/josepavese/needlex/internal/rendering"
)

// The serial MCP dispatcher owns one current service. Its bounded decision memo
// and semantic provider survive calls; changed configuration retires that state.
// Browser ownership stays with renderRuntime, borrowed by the service.
type serviceRuntime struct {
	key     [32]byte
	service *coreservice.Service
}

func (r *serviceRuntime) get(cfg config.Config, root string, renderer rendering.Renderer) (*coreservice.Service, error) {
	identity, err := json.Marshal([]any{cfg, root})
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256(identity)
	if r.service != nil && r.key == key {
		return r.service, nil
	}
	service, err := coreservice.NewWithRenderer(cfg, nil, root, renderer)
	if err == nil {
		r.key, r.service = key, service
	}
	return service, err
}
