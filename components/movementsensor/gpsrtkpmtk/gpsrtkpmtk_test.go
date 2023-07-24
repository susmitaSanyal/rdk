package gpsrtkpmtk

import (
	"context"
	"math"
	"testing"

	"github.com/edaniels/golog"
	geo "github.com/kellydunn/golang-geo"
	"go.viam.com/test"
	"go.viam.com/utils"

	"go.viam.com/rdk/components/movementsensor/fake"
	rtk "go.viam.com/rdk/components/movementsensor/rtkutils"
)

var (
	alt   = 50.5
	speed = 5.4
)

const (
	testRoverName   = "testRover"
	testStationName = "testStation"
	testBoardName   = "board1"
	testBusName     = "bus1"
	testi2cAddr     = 44
)

func TestValidateRTK(t *testing.T) {
	path := "path"
	cfg := Config{
		NtripURL:             "http//fakeurl",
		NtripConnectAttempts: 10,
		NtripPass:            "somepass",
		NtripUser:            "someuser",
		NtripMountpoint:      "NYC",
		Board:                testBoardName,
		I2CBus:               testBusName,
		I2CAddr:              testi2cAddr,
	}
	t.Run("valid config", func(t *testing.T) {
		err := cfg.validateNtrip(path)
		test.That(t, err, test.ShouldBeNil)
		err = cfg.validateI2C(path)
		test.That(t, err, test.ShouldBeNil)
		_, err = cfg.Validate(path)
		test.That(t, err, test.ShouldBeNil)
	})

	t.Run("invalid ntrip url", func(t *testing.T) {
		cfg := Config{
			NtripURL:             "",
			NtripConnectAttempts: 10,
			NtripPass:            "somepass",
			NtripUser:            "someuser",
			NtripMountpoint:      "NYC",
			Board:                testBoardName,
			I2CBus:               testBusName,
			I2CAddr:              testi2cAddr,
		}
		err := cfg.validateNtrip(path)
		test.That(t, err, test.ShouldBeError,
			utils.NewConfigValidationFieldRequiredError(path, "ntrip_url"))
		err = cfg.validateI2C(path)
		test.That(t, err, test.ShouldBeNil)

		_, err = cfg.Validate(path)
		test.That(t, err, test.ShouldBeError,
			utils.NewConfigValidationFieldRequiredError(path, "ntrip_url"))
	})

	t.Run("invalid i2c bus", func(t *testing.T) {
		cfg := Config{
			I2CBus:               "",
			NtripURL:             "http//fakeurl",
			NtripConnectAttempts: 10,
			NtripPass:            "somepass",
			NtripUser:            "someuser",
			NtripMountpoint:      "NYC",
			Board:                testBoardName,
			I2CAddr:              testi2cAddr,
		}
		err := cfg.validateNtrip(path)
		test.That(t, err, test.ShouldBeNil)
		err = cfg.validateI2C(path)
		test.That(t, err, test.ShouldBeError,
			utils.NewConfigValidationFieldRequiredError(path, "i2c_bus"))
		_, err = cfg.Validate(path)
		test.That(t, err, test.ShouldBeError,
			utils.NewConfigValidationFieldRequiredError(path, "i2c_bus"))
	})

	t.Run("invalid i2c addr", func(t *testing.T) {
		cfg := Config{
			I2CAddr:              0,
			NtripURL:             "http//fakeurl",
			NtripConnectAttempts: 10,
			NtripPass:            "somepass",
			NtripUser:            "someuser",
			NtripMountpoint:      "NYC",
			Board:                testBoardName,
			I2CBus:               testBusName,
		}
		err := cfg.validateNtrip(path)
		test.That(t, err, test.ShouldBeNil)
		err = cfg.validateI2C(path)
		test.That(t, err, test.ShouldBeError,
			utils.NewConfigValidationFieldRequiredError(path, "i2c_addr"))
		_, err = cfg.Validate(path)
		test.That(t, err, test.ShouldBeError,
			utils.NewConfigValidationFieldRequiredError(path, "i2c_addr"))
	})
}

func TestConnect(t *testing.T) {
	logger := golog.NewTestLogger(t)
	ctx := context.Background()
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	g := rtkI2C{
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
		logger:     logger,
	}

	url := "http://fakeurl"
	username := "user"
	password := "pwd"

	// create new ntrip client and connect
	err := g.connect("invalidurl", username, password, 10)
	test.That(t, err.Error(), test.ShouldContainSubstring, `Can't connect to NTRIP caster`)

	g.ntripClient = makeMockNtripClient()

	err = g.connect(url, username, password, 10)
	test.That(t, err, test.ShouldBeNil)

	err = g.getStream("", 10)
	test.That(t, err.Error(), test.ShouldContainSubstring, `no such host`)
}

func TestReadings(t *testing.T) {
	logger := golog.NewTestLogger(t)
	ctx := context.Background()
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	g := rtkI2C{
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
		logger:     logger,
	}

	mockSensor := &CustomMovementSensor{
		MovementSensor: &fake.MovementSensor{},
	}

	mockSensor.PositionFunc = func(ctx context.Context, extra map[string]interface{}) (*geo.Point, float64, error) {
		return geo.NewPoint(40.7, -73.98), 50.5, nil
	}

	g.nmeamovementsensor = mockSensor

	// Normal position
	loc1, alt1, err := g.Position(ctx, make(map[string]interface{}))
	test.That(t, err, test.ShouldBeNil)
	test.That(t, loc1, test.ShouldResemble, geo.NewPoint(40.7, -73.98))
	test.That(t, alt1, test.ShouldEqual, alt)

	speed1, err := g.LinearVelocity(ctx, make(map[string]interface{}))
	test.That(t, err, test.ShouldBeNil)
	test.That(t, speed1.Y, test.ShouldEqual, speed)
	test.That(t, speed1.X, test.ShouldEqual, 0)
	test.That(t, speed1.Z, test.ShouldEqual, 0)

	// Zero position with latitude 0 and longitude 0.
	mockSensor.PositionFunc = func(ctx context.Context, extra map[string]interface{}) (*geo.Point, float64, error) {
		return geo.NewPoint(0, 0), 0, nil
	}

	loc2, alt2, err := g.Position(ctx, make(map[string]interface{}))
	test.That(t, err, test.ShouldBeNil)
	test.That(t, loc2, test.ShouldResemble, geo.NewPoint(0, 0))
	test.That(t, alt2, test.ShouldEqual, 0)

	speed2, err := g.LinearVelocity(ctx, make(map[string]interface{}))
	test.That(t, err, test.ShouldBeNil)
	test.That(t, speed2.Y, test.ShouldEqual, speed)
	test.That(t, speed2.X, test.ShouldEqual, 0)
	test.That(t, speed2.Z, test.ShouldEqual, 0)

	// Position with NaN values.
	mockSensor.PositionFunc = func(ctx context.Context, extra map[string]interface{}) (*geo.Point, float64, error) {
		return geo.NewPoint(math.NaN(), math.NaN()), math.NaN(), nil
	}

	g.lastposition.SetLastPosition(loc1)

	loc3, alt3, err := g.Position(ctx, make(map[string]interface{}))
	test.That(t, err, test.ShouldBeNil)
	test.That(t, loc3, test.ShouldResemble, loc1)
	test.That(t, math.IsNaN(alt3), test.ShouldBeTrue)

	speed3, err := g.LinearVelocity(ctx, make(map[string]interface{}))
	test.That(t, err, test.ShouldBeNil)
	test.That(t, speed3.Y, test.ShouldEqual, speed)
}

func TestCloseRTK(t *testing.T) {
	logger := golog.NewTestLogger(t)
	ctx := context.Background()
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	g := rtkI2C{
		cancelCtx:  cancelCtx,
		cancelFunc: cancelFunc,
		logger:     logger,
	}
	g.ntripClient = &rtk.NtripInfo{}
	g.nmeamovementsensor = &fake.MovementSensor{}

	err := g.Close(ctx)
	test.That(t, err, test.ShouldBeNil)
}

type CustomMovementSensor struct {
	*fake.MovementSensor
	PositionFunc func(ctx context.Context, extra map[string]interface{}) (*geo.Point, float64, error)
}

func (c *CustomMovementSensor) Position(ctx context.Context, extra map[string]interface{}) (*geo.Point, float64, error) {
	if c.PositionFunc != nil {
		return c.PositionFunc(ctx, extra)
	}
	// Fallback to the default implementation if PositionFunc is not set.
	return c.MovementSensor.Position(ctx, extra)
}

// mock ntripinfo client.
func makeMockNtripClient() *rtk.NtripInfo {
	return &rtk.NtripInfo{}
}