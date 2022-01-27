// Package wheeled implements some bases, like a wheeled base.
package wheeled

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/edaniels/golog"
	"github.com/pkg/errors"
	"go.uber.org/multierr"
	"go.viam.com/utils"

	"go.viam.com/rdk/component/base"
	"go.viam.com/rdk/component/motor"
	"go.viam.com/rdk/config"
	"go.viam.com/rdk/registry"
	"go.viam.com/rdk/robot"
)

func init() {
	fourWheelComp := registry.Component{
		Constructor: func(
			ctx context.Context, r robot.Robot, config config.Component, logger golog.Logger,
		) (interface{}, error) {
			return CreateFourWheelBase(ctx, r, config, logger)
		},
	}
	wheeledBaseComp := registry.Component{
		Constructor: func(
			ctx context.Context, r robot.Robot, config config.Component, logger golog.Logger,
		) (interface{}, error) {
			return CreateWheeledBase(ctx, r, config, logger)
		},
	}
	registry.RegisterComponent(base.Subtype, "four-wheel", fourWheelComp)
	registry.RegisterComponent(base.Subtype, "wheeled", wheeledBaseComp)
}

type wheeledBase struct {
	widthMm              int
	wheelCircumferenceMm int
	spinSlipFactor       float64

	left      []motor.Motor
	right     []motor.Motor
	allMotors []motor.Motor
}

func (base *wheeledBase) Spin(ctx context.Context, angleDeg float64, degsPerSec float64, block bool) error {
	// Stop the motors if the speed is 0
	if math.Abs(degsPerSec) < 0.0001 {
		err := base.Stop(ctx)
		if err != nil {
			return errors.Errorf("error when trying to spin at a speed of 0: %v", err)
		}
		return err
	}

	// Spin math
	rpm, revolutions := base.spinMath(angleDeg, degsPerSec)

	// Send motor commands
	var err error
	for _, m := range base.left {
		err = multierr.Combine(err, m.GoFor(ctx, -rpm, revolutions))
	}
	for _, m := range base.right {
		err = multierr.Combine(err, m.GoFor(ctx, rpm, revolutions))
	}

	if err != nil {
		return multierr.Combine(err, base.Stop(ctx))
	}

	if !block {
		return nil
	}

	return base.WaitForMotorsToStop(ctx)
}

func (base *wheeledBase) MoveStraight(ctx context.Context, distanceMm int, mmPerSec float64, block bool) error {
	// Stop the motors if the speed or distance are 0
	if math.Abs(mmPerSec) < 0.0001 || distanceMm == 0 {
		err := base.Stop(ctx)
		if err != nil {
			return errors.Errorf("error when trying to move straight at a speed and/or distance of 0: %v", err)
		}
		return err
	}

	// Straight math
	rpm, rotations := base.straightDistanceToMotorInfo(distanceMm, mmPerSec)

	// Send motor commands
	for _, m := range base.allMotors {
		err := m.GoFor(ctx, rpm, rotations)
		if err != nil {
			return multierr.Combine(err, base.Stop(ctx))
		}
	}

	if !block {
		return nil
	}

	return base.WaitForMotorsToStop(ctx)
}

func (base *wheeledBase) MoveArc(ctx context.Context, distanceMm int, mmPerSec float64, angleDeg float64, block bool) error {
	// Stop the motors if the speed is 0
	if math.Abs(mmPerSec) < 0.0001 {
		err := base.Stop(ctx)
		if err != nil {
			return errors.Errorf("error when trying to arc at a speed of 0: %v", err)
		}
		return err
	}

	// Arc math
	rpmLR, revLR := base.arcMath(distanceMm, mmPerSec, angleDeg)

	// Send motor commands
	var err error
	for _, m := range base.left {
		err = multierr.Combine(err, m.GoFor(ctx, rpmLR[0], revLR[0]))
	}

	for _, m := range base.right {
		err = multierr.Combine(err, m.GoFor(ctx, rpmLR[1], revLR[1]))
	}

	if err != nil {
		return multierr.Combine(err, base.Stop(ctx))
	}

	if !block {
		return nil
	}

	return base.WaitForMotorsToStop(ctx)
}

// returns rpm, revolutions for a spin motion.
func (base *wheeledBase) spinMath(angleDeg float64, degsPerSec float64) (float64, float64) {
	wheelTravel := base.spinSlipFactor * float64(base.widthMm) * math.Pi * angleDeg / 360.0
	revolutions := wheelTravel / float64(base.wheelCircumferenceMm)

	// RPM = revolutions (unit) * deg/sec * (1 rot / 2pi deg) * (60 sec / 1 min) = rot/min
	rpm := revolutions * degsPerSec * 30 / math.Pi
	revolutions = math.Abs(revolutions)

	return rpm, revolutions
}

func (base *wheeledBase) arcMath(distanceMm int, mmPerSec float64, angleDeg float64) ([]float64, []float64) {
	// Spin the base if the distance is 0
	if distanceMm == 0 {
		rpm, revolutions := base.spinMath(angleDeg, mmPerSec)
		rpms := []float64{-rpm, rpm}
		rots := []float64{revolutions, revolutions}

		return rpms, rots
	}

	if distanceMm < 0 {
		distanceMm *= -1
		mmPerSec *= -1
	}

	// Base calculations
	v := mmPerSec
	t := float64(distanceMm) / mmPerSec
	r := float64(base.wheelCircumferenceMm) / (2.0 * math.Pi)
	l := float64(base.widthMm)

	degsPerSec := angleDeg / t
	w0 := degsPerSec / 180 * math.Pi
	wL := (v / r) - (l * w0 / (2 * r))
	wR := (v / r) + (l * w0 / (2 * r))

	// Calculate # of rotations
	rotL := wL * t / (2 * math.Pi)
	rotR := wR * t / (2 * math.Pi)

	// RPM = revolutions (unit) * deg/sec * (1 rot / 2pi deg) * (60 sec / 1 min) = rot/min
	rpmL := (wL / (2 * math.Pi)) * 60
	rpmR := (wR / (2 * math.Pi)) * 60

	rpms := []float64{rpmL, rpmR}
	rots := []float64{rotL, rotR}

	return rpms, rots
}

func (base *wheeledBase) straightDistanceToMotorInfo(distanceMm int, mmPerSec float64) (float64, float64) {
	rotations := float64(distanceMm) / float64(base.wheelCircumferenceMm)

	rotationsPerSec := mmPerSec / float64(base.wheelCircumferenceMm)
	rpm := 60 * rotationsPerSec

	return rpm, rotations
}

func (base *wheeledBase) WaitForMotorsToStop(ctx context.Context) error {
	for {
		if !utils.SelectContextOrWait(ctx, 10*time.Millisecond) {
			return ctx.Err()
		}

		anyOn := false
		anyOff := false

		for _, m := range base.allMotors {
			isOn, err := m.IsOn(ctx)
			if err != nil {
				return err
			}
			if isOn {
				anyOn = true
			} else {
				anyOff = true
			}
		}

		if !anyOn {
			return nil
		}

		if anyOff {
			// once one motor turns off, we turn them all off
			return base.Stop(ctx)
		}
	}
}

func (base *wheeledBase) Stop(ctx context.Context) error {
	var err error
	for _, m := range base.allMotors {
		err = multierr.Combine(err, m.Stop(ctx))
	}
	return err
}

func (base *wheeledBase) Close(ctx context.Context) error {
	return base.Stop(ctx)
}

func (base *wheeledBase) GetWidth(ctx context.Context) (int, error) {
	return base.widthMm, nil
}

// CreateFourWheelBase returns a new four wheel base defined by the given config.
func CreateFourWheelBase(ctx context.Context, r robot.Robot, config config.Component, logger golog.Logger) (base.LocalBase, error) {
	frontLeft, ok := r.MotorByName(config.Attributes.String("frontLeft"))
	if !ok {
		return nil, errors.New("frontLeft motor not found")
	}
	frontRight, ok := r.MotorByName(config.Attributes.String("frontRight"))
	if !ok {
		return nil, errors.New("frontRight motor not found")
	}
	backLeft, ok := r.MotorByName(config.Attributes.String("backLeft"))
	if !ok {
		return nil, errors.New("backLeft motor not found")
	}
	backRight, ok := r.MotorByName(config.Attributes.String("backRight"))
	if !ok {
		return nil, errors.New("backRight motor not found")
	}

	base := &wheeledBase{
		widthMm:              config.Attributes.Int("widthMm", 0),
		wheelCircumferenceMm: config.Attributes.Int("wheelCircumferenceMm", 0),
		spinSlipFactor:       config.Attributes.Float64("spinSlipFactor", 1.0),
		left:                 []motor.Motor{frontLeft, backLeft},
		right:                []motor.Motor{frontRight, backRight},
	}

	if base.widthMm == 0 {
		return nil, errors.New("need a widthMm for a four-wheel base")
	}

	if base.wheelCircumferenceMm == 0 {
		return nil, errors.New("need a wheelCircumferenceMm for a four-wheel base")
	}

	base.allMotors = append(base.allMotors, base.left...)
	base.allMotors = append(base.allMotors, base.right...)

	return base, nil
}

// CreateWheeledBase returns a new wheeled base defined by the given config.
func CreateWheeledBase(ctx context.Context, r robot.Robot, config config.Component, logger golog.Logger) (base.LocalBase, error) {
	base := &wheeledBase{
		widthMm:              config.Attributes.Int("widthMm", 0),
		wheelCircumferenceMm: config.Attributes.Int("wheelCircumferenceMm", 0),
		spinSlipFactor:       config.Attributes.Float64("spinSlipFactor", 1.0),
	}

	if base.widthMm == 0 {
		return nil, errors.New("need a widthMm for a wheeled base")
	}

	if base.wheelCircumferenceMm == 0 {
		return nil, errors.New("need a wheelCircumferenceMm for a wheeled base")
	}

	for _, name := range config.Attributes.StringSlice("left") {
		m, ok := r.MotorByName(name)
		if !ok {
			return nil, fmt.Errorf("no left motor named (%s)", name)
		}
		base.left = append(base.left, m)
	}

	for _, name := range config.Attributes.StringSlice("right") {
		m, ok := r.MotorByName(name)
		if !ok {
			return nil, fmt.Errorf("no right motor named (%s)", name)
		}
		base.right = append(base.right, m)
	}

	if len(base.left) == 0 {
		return nil, errors.New("need left and right motors")
	}

	if len(base.left) != len(base.right) {
		return nil, fmt.Errorf("left and right need to have the same number of motors, not %d vs %d", len(base.left), len(base.right))
	}

	base.allMotors = append(base.allMotors, base.left...)
	base.allMotors = append(base.allMotors, base.right...)

	return base, nil
}
