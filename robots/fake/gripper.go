package fake

import (
	"context"

	"github.com/edaniels/golog"

	"go.viam.com/robotcore/config"
	"go.viam.com/robotcore/gripper"
	"go.viam.com/robotcore/registry"
	"go.viam.com/robotcore/robot"
)

func init() {
	registry.RegisterGripper(ModelName, func(ctx context.Context, r robot.Robot, config config.Component, logger golog.Logger) (gripper.Gripper, error) {
		return &Gripper{Name: config.Name}, nil
	})
}

type Gripper struct {
	Name string
}

func (g *Gripper) Open(ctx context.Context) error {
	return nil
}

func (g *Gripper) Close() error {
	return nil
}

func (g *Gripper) Grab(ctx context.Context) (bool, error) {
	return false, nil
}
