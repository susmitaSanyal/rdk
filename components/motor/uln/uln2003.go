// Package gpiostepper implements a GPIO based stepper motor.
package uln

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/edaniels/golog"
	"github.com/pkg/errors"
	"go.viam.com/utils"

	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/generic"
	"go.viam.com/rdk/components/motor"
	"go.viam.com/rdk/config"
	"go.viam.com/rdk/operation"
	"go.viam.com/rdk/registry"
	"go.viam.com/rdk/resource"
	rdkutils "go.viam.com/rdk/utils"
)

var model = resource.NewDefaultModel("uln2003")

var step_sequence = [8][4]bool{{false, false, false, true},
	{false, false, true, true},
	{false, false, true, false},
	{false, true, true, false},
	{false, true, false, false},
	{true, true, false, false},
	{true, false, false, false},
	{true, false, false, true}}

// PinConfig defines the mapping of where motor are wired.
type PinConfig struct {
	In1 string `json:"In1"`
	In2 string `json:"In2"`
	In3 string `json:"In3"`
	In4 string `json:"In4"`
}

// Config describes the configuration of a motor.
type Config struct {
	Pins             PinConfig `json:"pins"`
	BoardName        string    `json:"board"`
	StepperDelay     uint      `json:"stepper_delay_usec,omitempty"` // When using stepper motors, the time to remain high
	TicksPerRotation int       `json:"ticks_per_rotation"`
}

func (config *Config) Validate(path string) ([]string, error) {
	var deps []string
	if config.BoardName == "" {
		return nil, utils.NewConfigValidationFieldRequiredError(path, "board")
	}
	deps = append(deps, config.BoardName)
	return deps, nil
}

func init() {
	_motor := registry.Component{
		Constructor: func(ctx context.Context, deps registry.Dependencies, config config.Component, logger golog.Logger) (interface{}, error) {
			actualBoard, motorConfig, err := getBoardFromRobotConfig(deps, config)
			if err != nil {
				return nil, err
			}

			return newULN(ctx, actualBoard, *motorConfig, config.Name, logger)
		},
	}
	registry.RegisterComponent(motor.Subtype, model, _motor)
	config.RegisterComponentAttributeMapConverter(
		motor.Subtype,
		model,
		func(attributes config.AttributeMap) (interface{}, error) {
			var conf Config
			return config.TransformAttributeMapToStruct(&conf, attributes)
		},
		&Config{},
	)
}

func getBoardFromRobotConfig(deps registry.Dependencies, config config.Component) (board.Board, *Config, error) {
	motorConfig, ok := config.ConvertedAttributes.(*Config)
	if !ok {
		return nil, nil, rdkutils.NewUnexpectedTypeError(motorConfig, config.ConvertedAttributes)
	}
	if motorConfig.BoardName == "" {
		return nil, nil, errors.New("expected board name in config for motor")
	}
	b, err := board.FromDependencies(deps, motorConfig.BoardName)
	if err != nil {
		return nil, nil, err
	}
	return b, motorConfig, nil
}

func newULN(ctx context.Context, b board.Board, mc Config, name string, logger golog.Logger) (motor.Motor, error) {
	if mc.TicksPerRotation == 0 {
		return nil, errors.New("expected ticks_per_rotation in config for motor")
	}

	m := &uln2003{
		theBoard:         b,
		ticksPerRotation: mc.TicksPerRotation,
		stepperDelay:     mc.StepperDelay,
		logger:           logger,
		motorName:        name,
	}

	if mc.Pins.In1 != "" {
		in1, err := b.GPIOPinByName(mc.Pins.In1)
		if err != nil {
			return nil, err
		}
		m.in1 = in1
	}

	if mc.Pins.In2 != "" {
		in2, err := b.GPIOPinByName(mc.Pins.In2)
		if err != nil {
			return nil, err
		}
		m.in2 = in2
	}

	if mc.Pins.In3 != "" {
		in3, err := b.GPIOPinByName(mc.Pins.In3)
		if err != nil {
			return nil, err
		}
		m.in3 = in3
	}

	if mc.Pins.In4 != "" {
		in4, err := b.GPIOPinByName(mc.Pins.In4)
		if err != nil {
			return nil, err
		}
		m.in4 = in4
	}

	if err := m.Validate(); err != nil {
		return nil, err
	}

	m.startThread(ctx)
	return m, nil
}

type uln2003 struct {
	theBoard           board.Board
	ticksPerRotation   int
	stepperDelay       uint
	in1, in2, in3, in4 board.GPIOPin
	logger             golog.Logger
	motorName          string

	// state
	lock  sync.Mutex
	opMgr operation.SingleOperationManager

	stepPosition         int64
	threadStarted        bool
	targetStepPosition   int64
	targetStepsPerSecond int64
	generic.Unimplemented
}

// validate if this config is valid.
func (m *uln2003) Validate() error {

	if m.theBoard == nil {
		return errors.New("need a board for uln2003")
	}

	if m.ticksPerRotation == 0 {
		return errors.New("need to set 'ticks per rotation' for uln2003")
	}

	if m.stepperDelay == 0 {
		m.stepperDelay = 20
	}

	if m.in1 == nil {
		return errors.New("need a 'In1' pin for uln2003")
	}

	if m.in2 == nil {
		return errors.New("need a 'In2' pin for uln2003")
	}

	if m.in3 == nil {
		return errors.New("need a 'In3' pin for uln2003")
	}

	if m.in4 == nil {
		return errors.New("need a 'In4' pin for uln2003")
	}

	return nil
}

// SetPower sets the percentage of power the motor should employ between 0-1.
func (m *uln2003) SetPower(ctx context.Context, powerPct float64, extra map[string]interface{}) error {
	if math.Abs(powerPct) <= .0001 {
		m.stop()
		return nil
	}
	return errors.Errorf("doesn't support raw power mode in motor (%s)", m.motorName)
}

func (m *uln2003) startThread(ctx context.Context) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.threadStarted {
		return
	}

	m.threadStarted = true
	go m.doRun(ctx)
}

func (m *uln2003) doRun(ctx context.Context) {
	for {
		sleep, err := m.doCycle(ctx)
		if err != nil {
			m.logger.Info("error in uln2003 %w", err)
		}

		if !utils.SelectContextOrWait(ctx, sleep) {
			return
		}
	}
}

func (m *uln2003) doCycle(ctx context.Context) (time.Duration, error) {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.stepPosition == m.targetStepPosition {
		return 5 * time.Millisecond, nil
	}

	err := m.doStep(ctx, m.stepPosition < m.targetStepPosition)
	if err != nil {
		return time.Second, fmt.Errorf("error stepping %w", err)
	}

	return time.Duration(int64(time.Microsecond*1000*1000) / int64(math.Abs(float64(m.targetStepsPerSecond)))), nil
}

// have to be locked to call.
func (m *uln2003) doStep(ctx context.Context, forward bool) error {

	time.Sleep(time.Duration(m.stepperDelay) * time.Microsecond)

	if forward {
		for steps := 0; steps < len(step_sequence); steps++ {
			m.in1.Set(ctx, step_sequence[steps][0], nil)
			m.in2.Set(ctx, step_sequence[steps][1], nil)
			m.in3.Set(ctx, step_sequence[steps][2], nil)
			m.in4.Set(ctx, step_sequence[steps][3], nil)

		}
		m.stepPosition++

	} else {
		for steps := len(step_sequence); steps >= 0; steps-- {
			m.in1.Set(ctx, step_sequence[steps][0], nil)
			m.in2.Set(ctx, step_sequence[steps][1], nil)
			m.in3.Set(ctx, step_sequence[steps][2], nil)
			m.in4.Set(ctx, step_sequence[steps][3], nil)

		}
		m.stepPosition--

	}

	return nil
}

// GoFor instructs the motor to go in a specific direction for a specific amount of
// revolutions at a given speed in revolutions per minute. Both the RPM and the revolutions
// can be assigned negative values to move in a backwards direction. Note: if both are negative
// the motor will spin in the forward direction.
func (m *uln2003) GoFor(ctx context.Context, rpm, revolutions float64, extra map[string]interface{}) error {
	if rpm == 0 {
		return motor.NewZeroRPMError()
	}

	ctx, done := m.opMgr.New(ctx)
	defer done()

	err := m.goForInternal(ctx, rpm, revolutions)
	if err != nil {
		return errors.Wrapf(err, "error in GoFor from motor (%s)", m.motorName)
	}

	if revolutions == 0 {
		return nil
	}

	return m.opMgr.WaitTillNotPowered(ctx, time.Millisecond, m, m.Stop)
}

func (m *uln2003) goForInternal(ctx context.Context, rpm, revolutions float64) error {
	if revolutions == 0 {
		revolutions = 1000000.0
	}
	var d int64 = 1

	if math.Signbit(revolutions) != math.Signbit(rpm) {
		d = -1
	}

	revolutions = math.Abs(revolutions)
	rpm = math.Abs(rpm) * float64(d)

	if math.Abs(rpm) < 0.1 {
		return m.Stop(ctx, nil)
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	if !m.threadStarted {
		return errors.New("thread not started")
	}

	m.targetStepPosition += int64(float64(d) * revolutions * float64(m.ticksPerRotation))
	m.targetStepsPerSecond = int64(rpm * float64(m.ticksPerRotation) / 60.0)
	if m.targetStepsPerSecond == 0 {
		m.targetStepsPerSecond = 1
	}

	return nil
}

// GoTo instructs the motor to go to a specific position (provided in revolutions from home/zero),
// at a specific RPM. Regardless of the directionality of the RPM this function will move the motor
// towards the specified target.
func (m *uln2003) GoTo(ctx context.Context, rpm, positionRevolutions float64, extra map[string]interface{}) error {
	curPos, err := m.Position(ctx, extra)
	if err != nil {
		return errors.Wrapf(err, "error in GoTo from motor (%s)", m.motorName)
	}
	moveDistance := positionRevolutions - curPos

	return m.GoFor(ctx, math.Abs(rpm), moveDistance, extra)
}

// Set the current position (+/- offset) to be the new zero (home) position.
func (m *uln2003) ResetZeroPosition(ctx context.Context, offset float64, extra map[string]interface{}) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.stepPosition = int64(offset * float64(m.ticksPerRotation))
	return nil
}

// Position reports the position of the motor based on its encoder. If it's not supported, the returned
// data is undefined. The unit returned is the number of revolutions which is intended to be fed
// back into calls of GoFor.
func (m *uln2003) Position(ctx context.Context, extra map[string]interface{}) (float64, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return float64(m.stepPosition) / float64(m.ticksPerRotation), nil
}

// Properties returns the status of whether the motor supports certain optional features.
func (m *uln2003) Properties(ctx context.Context, extra map[string]interface{}) (map[motor.Feature]bool, error) {
	return map[motor.Feature]bool{
		motor.PositionReporting: true,
	}, nil
}

// IsMoving returns if the motor is currently moving.
func (m *uln2003) IsMoving(ctx context.Context) (bool, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.stepPosition != m.targetStepPosition, nil
}

// Stop turns the power to the motor off immediately, without any gradual step down.
func (m *uln2003) Stop(ctx context.Context, extra map[string]interface{}) error {
	m.stop()
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.enable(ctx, false)
}

func (m *uln2003) stop() {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.targetStepPosition = m.stepPosition
	m.targetStepsPerSecond = 0
}

// IsPowered returns whether or not the motor is currently on. It also returns the percent power
// that the motor has, but stepper motors only have this set to 0% or 100%, so it's a little
// redundant.
func (m *uln2003) IsPowered(ctx context.Context, extra map[string]interface{}) (bool, float64, error) {
	on, err := m.IsMoving(ctx)
	if err != nil {
		return on, 0.0, errors.Wrapf(err, "error in IsPowered from motor (%s)", m.motorName)
	}
	percent := 0.0
	if on {
		percent = 1.0
	}
	return on, percent, err
}

func (m *uln2003) enable(ctx context.Context, on bool) error {
	if m.in1 != nil {
		return m.in1.Set(ctx, !on, nil)
	}

	if m.in2 != nil {
		return m.in2.Set(ctx, !on, nil)
	}

	if m.in3 != nil {
		return m.in3.Set(ctx, !on, nil)
	}

	if m.in2 != nil {
		return m.in4.Set(ctx, !on, nil)
	}

	return nil
}
