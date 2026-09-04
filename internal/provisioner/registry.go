package provisioner

import (
	"fmt"

	"github.com/dianrp/drp-billing/internal/auth"
	"github.com/dianrp/drp-billing/internal/provision"
	"github.com/dianrp/drp-billing/internal/provision/radius"
	"github.com/dianrp/drp-billing/internal/provision/routeros"
	"github.com/dianrp/drp-billing/internal/store"
)

type Registry struct {
	routeros *routeros.Client
	radius   *radius.Client
}

func NewRegistry(st *store.Store, enc *auth.Encryptor) *Registry {
	return &Registry{
		routeros: routeros.New(st, enc),
		radius:   radius.New(st),
	}
}

func (r *Registry) Get(provisionerType string) (provision.Provisioner, error) {
	switch provisionerType {
	case "routeros", "":
		return r.routeros, nil
	case "radius":
		return r.radius, nil
	default:
		return nil, fmt.Errorf("unknown provisioner: %s", provisionerType)
	}
}
