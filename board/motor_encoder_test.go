package board

import (
	"context"
	"sync"
	"testing"

	"go.viam.com/utils"
	"go.viam.com/utils/testutils"

	pb "go.viam.com/core/proto/api/v1"
	"go.viam.com/core/rlog"

	"github.com/edaniels/golog"
	"go.viam.com/test"
)

func TestMotorEncoder1(t *testing.T) {
	logger := golog.NewTestLogger(t)
	undo := setRPMSleepDebug(1, false)
	defer undo()

	cfg := MotorConfig{TicksPerRotation: 100}
	real := &FakeMotor{mu: &sync.Mutex{}}
	interrupt := &BasicDigitalInterrupt{}
	encoder := &singleEncoder{i: interrupt}

	motor, err := newEncodedMotor(cfg, real, encoder, logger)
	test.That(t, err, test.ShouldBeNil)
	defer func() {
		test.That(t, motor.Close(), test.ShouldBeNil)
	}()
	encoder.m = motor

	// test some basic defaults
	isOn, err := motor.IsOn(context.Background())
	test.That(t, err, test.ShouldBeNil)
	test.That(t, isOn, test.ShouldBeFalse)
	supported, err := motor.PositionSupported(context.Background())
	test.That(t, err, test.ShouldBeNil)
	test.That(t, supported, test.ShouldBeTrue)

	test.That(t, motor.isRegulated(), test.ShouldBeFalse)
	motor.setRegulated(true)
	test.That(t, motor.isRegulated(), test.ShouldBeTrue)
	motor.setRegulated(false)
	test.That(t, motor.isRegulated(), test.ShouldBeFalse)

	// when we go forward things work
	test.That(t, motor.Go(context.Background(), pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD, .01), test.ShouldBeNil)
	test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD)
	test.That(t, real.PowerPct(), test.ShouldEqual, float32(.01))

	// stop
	test.That(t, motor.Off(context.Background()), test.ShouldBeNil)
	test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_UNSPECIFIED)

	// now test basic control
	test.That(t, motor.GoFor(context.Background(), pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD, 1000, 1), test.ShouldBeNil)
	test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD)
	test.That(t, real.PowerPct(), test.ShouldBeGreaterThan, float32(0))

	testutils.WaitForAssertion(t, func(t testing.TB) {
		test.That(t, motor.RPMMonitorCalls(), test.ShouldBeGreaterThan, int64(10))
		test.That(t, real.PowerPct(), test.ShouldEqual, float32(1))
	})

	interrupt.ticks(99, nowNanosTest())
	test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD)
	interrupt.Tick(true, nowNanosTest())

	testutils.WaitForAssertion(t, func(t testing.TB) {
		test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_UNSPECIFIED)
	})

	// when we're in the middle of a GoFor and then call Go, don't turn off
	test.That(t, motor.GoFor(context.Background(), pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD, 1000, 1), test.ShouldBeNil)
	test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD)
	test.That(t, real.PowerPct(), test.ShouldBeGreaterThan, float32(0))

	testutils.WaitForAssertion(t, func(t testing.TB) {
		test.That(t, real.PowerPct(), test.ShouldEqual, float32(1))
	})

	// we didn't hit the set point
	interrupt.ticks(99, nowNanosTest())
	test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD)

	// go to non controlled
	motor.Go(context.Background(), pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD, .25)

	// go far!
	interrupt.ticks(1000, nowNanosTest())

	// we should still be moving at the previous force
	testutils.WaitForAssertion(t, func(t testing.TB) {
		test.That(t, real.PowerPct(), test.ShouldEqual, float32(.25))
		test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD)
	})

	test.That(t, motor.Off(context.Background()), test.ShouldBeNil)

	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos, err := motor.Position(context.Background())
		test.That(t, err, test.ShouldBeNil)
		test.That(t, pos, test.ShouldEqual, 11.99)
	})

	// same thing, but backwards
	test.That(t, motor.GoFor(context.Background(), pb.DirectionRelative_DIRECTION_RELATIVE_BACKWARD, 1000, 1), test.ShouldBeNil)
	test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_BACKWARD)
	test.That(t, real.PowerPct(), test.ShouldBeGreaterThan, float32(0))

	testutils.WaitForAssertion(t, func(t testing.TB) {
		test.That(t, motor.RPMMonitorCalls(), test.ShouldBeGreaterThan, int64(10))
		test.That(t, real.PowerPct(), test.ShouldEqual, float32(1))
	})

	interrupt.ticks(99, nowNanosTest())
	testutils.WaitForAssertion(t, func(t testing.TB) {
		test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_BACKWARD)
	})
	interrupt.Tick(true, nowNanosTest())
	testutils.WaitForAssertion(t, func(t testing.TB) {
		test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_UNSPECIFIED)
	})

	// test go for without a rotation limit
	test.That(t, motor.GoFor(context.Background(), pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD, 1000, 0), test.ShouldBeNil)
	atStart := motor.RPMMonitorCalls()
	testutils.WaitForAssertion(t, func(t testing.TB) {
		test.That(t, motor.RPMMonitorCalls(), test.ShouldBeGreaterThan, atStart+10)
		test.That(t, real.PowerPct(), test.ShouldBeGreaterThan, float32(.5))
	})
	test.That(t, motor.Off(context.Background()), test.ShouldBeNil)

}

func TestMotorEncoderHall(t *testing.T) {
	logger := golog.NewTestLogger(t)
	undo := setRPMSleepDebug(1, false)
	defer undo()

	cfg := MotorConfig{TicksPerRotation: 100}
	real := &FakeMotor{mu: &sync.Mutex{}}
	encoderA := &BasicDigitalInterrupt{}
	encoderB := &BasicDigitalInterrupt{}
	encoder := NewHallEncoder(encoderA, encoderB)

	motor, err := newEncodedMotor(cfg, real, encoder, logger)
	test.That(t, err, test.ShouldBeNil)
	defer func() {
		test.That(t, motor.Close(), test.ShouldBeNil)
	}()

	motor.rpmMonitorStart()
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 0)
	})

	encoderA.Tick(true, nowNanosTest()) // this should do nothing because it's the initial state
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 0)
	})

	encoderB.Tick(false, nowNanosTest()) // we go from state 3 -> 4
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 1)
	})

	encoderB.Tick(false, nowNanosTest()) // bounce, we should do nothing
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 1)
	})

	encoderA.Tick(false, nowNanosTest()) // 4 -> 1
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 2)
	})

	encoderB.Tick(true, nowNanosTest()) // 1 -> 2
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 3)
	})

	encoderA.Tick(true, nowNanosTest()) // 2- -> 3
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 4)
	})

	// start going backwards
	encoderA.Tick(false, nowNanosTest()) // 3 -> 2
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 3)
	})

	encoderB.Tick(false, nowNanosTest()) // 2 -> 1
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 2)
	})

	encoderA.Tick(true, nowNanosTest()) // 1 -> 4
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 1)
	})

	encoderB.Tick(true, nowNanosTest()) // 4 -> 1
	testutils.WaitForAssertion(t, func(t testing.TB) {
		pos := encoder.rawPosition()
		test.That(t, pos, test.ShouldEqual, 0)
	})

	// do a GoFor and make sure we stop
	t.Run("GoFor", func(t *testing.T) {
		undo := setRPMSleepDebug(1, false)
		defer undo()

		err := motor.GoFor(context.Background(), pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD, 100, 1)
		test.That(t, err, test.ShouldBeNil)
		test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD)

		testutils.WaitForAssertion(t, func(t testing.TB) {
			test.That(t, real.PowerPct(), test.ShouldEqual, 1.0)
		})

		for x := 0; x < 100; x++ {
			encoderB.Tick(true, nowNanosTest())
			encoderA.Tick(true, nowNanosTest())
			encoderB.Tick(false, nowNanosTest())
			encoderA.Tick(false, nowNanosTest())
		}

		testutils.WaitForAssertion(t, func(t testing.TB) {
			test.That(t, real.Direction(), test.ShouldNotEqual, pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD)
		})

	})

	t.Run("GoFor-backwards", func(t *testing.T) {
		//undo := setRPMSleepDebug(1, false)
		//defer undo()

		err := motor.GoFor(context.Background(), pb.DirectionRelative_DIRECTION_RELATIVE_BACKWARD, 100, 1)
		test.That(t, err, test.ShouldBeNil)
		test.That(t, real.Direction(), test.ShouldEqual, pb.DirectionRelative_DIRECTION_RELATIVE_BACKWARD)

		testutils.WaitForAssertion(t, func(t testing.TB) {
			test.That(t, real.PowerPct(), test.ShouldEqual, 1.0)
		})

		for x := 0; x < 100; x++ {
			encoderA.Tick(false, nowNanosTest())
			encoderB.Tick(false, nowNanosTest())
			encoderA.Tick(true, nowNanosTest())
			encoderB.Tick(true, nowNanosTest())
		}

		testutils.WaitForAssertion(t, func(t testing.TB) {
			test.That(t, real.Direction(), test.ShouldNotEqual, pb.DirectionRelative_DIRECTION_RELATIVE_FORWARD)
		})

	})

}

func TestWrapMotorWithEncoder(t *testing.T) {
	logger := golog.NewTestLogger(t)
	real := &FakeMotor{mu: &sync.Mutex{}}

	// don't wrap with no encoder
	m, err := WrapMotorWithEncoder(context.Background(), nil, MotorConfig{}, real, logger)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, m, test.ShouldEqual, real)
	test.That(t, utils.TryClose(m), test.ShouldBeNil)

	// enforce need TicksPerRotation
	m, err = WrapMotorWithEncoder(context.Background(), nil, MotorConfig{Encoder: "a"}, real, logger)
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, m, test.ShouldBeNil)
	test.That(t, utils.TryClose(m), test.ShouldBeNil)

	b, err := NewFakeBoard(context.Background(), Config{}, rlog.Logger)
	test.That(t, err, test.ShouldBeNil)

	// enforce need encoder
	m, err = WrapMotorWithEncoder(context.Background(), b, MotorConfig{Encoder: "a", TicksPerRotation: 100}, real, logger)
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, m, test.ShouldBeNil)
	test.That(t, utils.TryClose(m), test.ShouldBeNil)

	b.digitals["a"] = &BasicDigitalInterrupt{}
	m, err = WrapMotorWithEncoder(context.Background(), b, MotorConfig{Encoder: "a", TicksPerRotation: 100}, real, logger)
	test.That(t, err, test.ShouldBeNil)
	_, ok := m.(*encodedMotor)
	test.That(t, ok, test.ShouldBeTrue)
	test.That(t, utils.TryClose(m), test.ShouldBeNil)

	// enforce need encoder b
	m, err = WrapMotorWithEncoder(context.Background(), b, MotorConfig{Encoder: "a", TicksPerRotation: 100, EncoderB: "b"}, real, logger)
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, m, test.ShouldBeNil)
	test.That(t, utils.TryClose(m), test.ShouldBeNil)

	m, err = WrapMotorWithEncoder(context.Background(), b, MotorConfig{Encoder: "a", EncoderB: "b", TicksPerRotation: 100}, real, logger)
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, m, test.ShouldBeNil)
	test.That(t, utils.TryClose(m), test.ShouldBeNil)
}

func TestWrapMotorWithEncoderRampMath(t *testing.T) {
	m := encodedMotor{rampRate: 0.5}

	test.That(t, m.computeRamp(0, 1), test.ShouldEqual, .5)
	test.That(t, m.computeRamp(0.5, 1), test.ShouldEqual, .75)

	m.rampRate = 1
	test.That(t, m.computeRamp(0, 1), test.ShouldEqual, 1)
	test.That(t, m.computeRamp(0.5, 1), test.ShouldEqual, 1)
	test.That(t, m.computeRamp(0.5, .9), test.ShouldEqual, .9)

	m.rampRate = .25
	test.That(t, m.computeRamp(0, 1), test.ShouldEqual, .25)
	test.That(t, m.computeRamp(0.5, 1), test.ShouldEqual, .625)
	test.That(t, m.computeRamp(0.999, 1), test.ShouldEqual, 1)

	test.That(t, m.computeRamp(.8-(1.0/255.0), .8), test.ShouldEqual, .8)
	test.That(t, m.computeRamp(.8+(1.0/255.0), .8), test.ShouldEqual, .8)
	test.That(t, m.computeRamp(.8-(2.0/255.0), .8), test.ShouldAlmostEqual, .7941176, .0000001)
	test.That(t, m.computeRamp(.8+(2.0/255.0), .8), test.ShouldAlmostEqual, .8058823, .0000001)

}
