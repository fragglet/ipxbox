package server

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/fragglet/ipxbox/module"
	"github.com/fragglet/ipxbox/server"
	"github.com/fragglet/ipxbox/server/dosbox"
	"github.com/fragglet/ipxbox/server/uplink"
)

type mod struct {
	port           *int
	clientTimeout  *time.Duration
	uplinkPassword *string
}

var (
	Module = (module.Module)(&mod{})
)

func (m *mod) Initialize() {
	m.port = flag.Int("port", 10000, "UDP port to listen on.")
	m.clientTimeout = flag.Duration("client_timeout", 10*time.Minute, "Time of inactivity before disconnecting clients.")
	m.uplinkPassword = flag.String("uplink_password", "", "Password to permit uplink clients to connect. If empty, uplink is not supported.")
}

func (m *mod) IsEnabled() bool {
	return true
}

func (m *mod) Start(ctx context.Context, params *module.Parameters) error {
	protocols := []server.Protocol{
		&dosbox.Protocol{
			Logger:        params.Logger,
			Network:       params.Network,
			KeepaliveTime: 5 * time.Second,
		},
	}
	if *m.uplinkPassword != "" {
		if params.Uplinkable == nil {
			return fmt.Errorf("Sorry, a direct connection is needed to run an uplink server.")
		}
		protocols = append(protocols, &uplink.Protocol{
			Logger:        params.Logger,
			Network:       params.Uplinkable,
			Password:      *m.uplinkPassword,
			KeepaliveTime: 5 * time.Second,
		})
	}
	s, err := server.New(fmt.Sprintf(":%d", *m.port), &server.Config{
		Protocols:     protocols,
		ClientTimeout: *m.clientTimeout,
		Logger:        params.Logger,
	})
	if err != nil {
		return err
	}
	s.Run(ctx)
	return nil
}
