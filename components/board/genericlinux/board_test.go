//go:build linux

// These tests will only run on Linux! Viam's automated build system on Github uses Linux, though,
// so they should run on every PR. We made the tests Linux-only because this entire package is
// Linux-only, and building non-Linux support solely for the test meant that the code tested might
// not be the production code.
package genericlinux

import (
	"context"
	"testing"

	"go.viam.com/test"

	"go.viam.com/rdk/components/board"
	"go.viam.com/rdk/components/board/mcp3008helper"
	"go.viam.com/rdk/logging"
	"go.viam.com/rdk/resource"
)

func TestGenericLinux(t *testing.T) {
	b := &Board{
		logger: logging.NewTestLogger(t),
	}

	t.Run("test empty sysfs board", func(t *testing.T) {
		_, err := b.GPIOPinByName("10")
		test.That(t, err, test.ShouldNotBeNil)
	})

	b = &Board{
		Named:         board.Named("foo").AsNamed(),
		gpioMappings:  nil,
		analogReaders: map[string]*wrappedAnalogReader{"an": {}},
		logger:        logging.NewTestLogger(t),
	}

	t.Run("test analog-readers digital-interrupts and gpio names", func(t *testing.T) {
		an1, err := b.AnalogByName("an")
		test.That(t, an1, test.ShouldHaveSameTypeAs, &wrappedAnalogReader{})
		test.That(t, err, test.ShouldBeNil)

		an2, err := b.AnalogByName("missing")
		test.That(t, an2, test.ShouldBeNil)
		test.That(t, err, test.ShouldNotBeNil)

		dn1, err := b.DigitalInterruptByName("dn")
		test.That(t, dn1, test.ShouldBeNil)
		test.That(t, err, test.ShouldNotBeNil)

		gn1, err := b.GPIOPinByName("10")
		test.That(t, err, test.ShouldNotBeNil)
		test.That(t, gn1, test.ShouldBeNil)
	})
}

func TestConfigValidate(t *testing.T) {
	validConfig := Config{}

	validConfig.AnalogReaders = []mcp3008helper.MCP3008AnalogConfig{{}}
	_, _, err := validConfig.Validate("path")
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, err.Error(), test.ShouldContainSubstring, `path.analogs.0`)
	test.That(t, resource.GetFieldFromFieldRequiredError(err), test.ShouldEqual, "name")

	validConfig.AnalogReaders = []mcp3008helper.MCP3008AnalogConfig{{Name: "bar"}}
	_, _, err = validConfig.Validate("path")
	test.That(t, err, test.ShouldBeNil)

	validConfig.DigitalInterrupts = []board.DigitalInterruptConfig{{}}
	_, _, err = validConfig.Validate("path")
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, err.Error(), test.ShouldContainSubstring, `path.digital_interrupts.0`)
	test.That(t, resource.GetFieldFromFieldRequiredError(err), test.ShouldEqual, "name")

	validConfig.DigitalInterrupts = []board.DigitalInterruptConfig{{Name: "bar"}}
	_, _, err = validConfig.Validate("path")
	test.That(t, err, test.ShouldNotBeNil)
	test.That(t, err.Error(), test.ShouldContainSubstring, `path.digital_interrupts.0`)
	test.That(t, resource.GetFieldFromFieldRequiredError(err), test.ShouldEqual, "pin")

	validConfig.DigitalInterrupts = []board.DigitalInterruptConfig{{Name: "bar", Pin: "3"}}
	_, _, err = validConfig.Validate("path")
	test.That(t, err, test.ShouldBeNil)
}

func TestNewBoard(t *testing.T) {
	logger := logging.NewTestLogger(t)
	ctx := context.Background()

	// Create a fake board mapping with two pins for testing.
	testBoardMappings := make(map[string]GPIOBoardMapping, 2)
	testBoardMappings["1"] = GPIOBoardMapping{
		GPIOChipDev:    "gpiochip0",
		GPIO:           1,
		GPIOName:       "1",
		PWMSysFsDir:    "",
		PWMID:          -1,
		HWPWMSupported: false,
	}
	testBoardMappings["2"] = GPIOBoardMapping{
		GPIOChipDev:    "gpiochip0",
		GPIO:           2,
		GPIOName:       "2",
		PWMSysFsDir:    "pwm.00",
		PWMID:          1,
		HWPWMSupported: true,
	}

	conf := &Config{}
	conf.AnalogReaders = []mcp3008helper.MCP3008AnalogConfig{{Name: "an1", Channel: "1"}}

	config := resource.Config{
		Name:                "board1",
		ConvertedAttributes: conf,
	}
	b, err := NewBoard(ctx, config, ConstPinDefs(testBoardMappings), logger)
	test.That(t, err, test.ShouldBeNil)
	test.That(t, b, test.ShouldNotBeNil)
	defer b.Close(ctx)

	gn1, err := b.GPIOPinByName("1")
	test.That(t, err, test.ShouldBeNil)
	// Our test framework uses reflection to walk the structs it asserts on. However, gn1 (and
	// later gn2) contains a mutex that was locked and unlocked by a background goroutine when it
	// was constructed at the beginning of the test. That was so recent that the Go runtime
	// environment will think there is a race condition when the test framework walks that part of
	// the struct. To avoid that, we don't use the test framework directly here.
	if gn1 == nil {
		t.FailNow()
	}

	gn2, err := b.GPIOPinByName("2")
	test.That(t, err, test.ShouldBeNil)
	if gn2 == nil {
		t.FailNow()
	}
}
