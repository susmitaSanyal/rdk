package status_test

import (
	"context"
	"errors"
	"testing"

	"go.viam.com/core/arm"
	"go.viam.com/core/base"
	"go.viam.com/core/board"
	"go.viam.com/core/gripper"
	"go.viam.com/core/lidar"
	pb "go.viam.com/core/proto/api/v1"
	"go.viam.com/core/robot"
	"go.viam.com/core/robots/fake"
	"go.viam.com/core/sensor"
	"go.viam.com/core/status"
	"go.viam.com/core/testutils/inject"

	"github.com/edaniels/golog"
	"github.com/edaniels/gostream"
	"go.viam.com/test"
)

func setupInjectRobotHelper(logger golog.Logger, withRemotes, refreshFail, isRemote bool) *inject.Robot {
	injectRobot := &inject.Robot{}

	injectRobot.ArmNamesFunc = func() []string {
		return []string{"arm1", "arm2"}
	}
	injectRobot.GripperNamesFunc = func() []string {
		return []string{"gripper1", "gripper2"}
	}
	injectRobot.CameraNamesFunc = func() []string {
		return []string{"camera1", "camera2"}
	}
	injectRobot.LidarNamesFunc = func() []string {
		return []string{"lidar1", "lidar2"}
	}
	injectRobot.BaseNamesFunc = func() []string {
		return []string{"base1", "base2"}
	}
	injectRobot.BoardNamesFunc = func() []string {
		return []string{"board1", "board2"}
	}
	injectRobot.SensorNamesFunc = func() []string {
		return []string{"sensor1", "sensor2"}
	}
	injectRobot.LoggerFunc = func() golog.Logger {
		return logger
	}

	injectRobot.ArmByNameFunc = func(name string) arm.Arm {
		return &fake.Arm{Name: name}
	}
	injectRobot.BaseByNameFunc = func(name string) base.Base {
		return &fake.Base{Name: name}
	}
	injectRobot.GripperByNameFunc = func(name string) gripper.Gripper {
		return &fake.Gripper{Name: name}
	}
	injectRobot.CameraByNameFunc = func(name string) gostream.ImageSource {
		return &fake.Camera{Name: name}
	}
	injectRobot.LidarByNameFunc = func(name string) lidar.Lidar {
		return &fake.Lidar{Name: name}
	}
	injectRobot.BoardByNameFunc = func(name string) board.Board {
		return &board.FakeBoard{Name: name}
	}
	injectRobot.SensorByNameFunc = func(name string) sensor.Sensor {
		return &fake.Compass{Name: name}
	}

	if withRemotes {
		injectRobot.RemoteNamesFunc = func() []string {
			return []string{"remote1", "remote2"}
		}
		remote1 := setupInjectRobotHelper(logger, false, false, true)
		remote2 := setupInjectRobotHelper(logger, false, refreshFail, true)
		injectRobot.RemoteByNameFunc = func(name string) robot.Robot {
			switch name {
			case "remote1":
				return remote1
			case "remote2":
				return remote2
			}
			return nil
		}
	} else {
		injectRobot.RemoteNamesFunc = func() []string {
			return nil
		}
	}

	injectRobot.RefreshFunc = func(ctx context.Context) error {
		if isRemote && refreshFail {
			return errors.New("whoops")
		}
		return nil
	}

	return injectRobot
}

func setupInjectRobot(logger golog.Logger, withRemotes, refreshFail bool) *inject.Robot {
	return setupInjectRobotHelper(logger, withRemotes, refreshFail, false)
}

func TestCreateStatus(t *testing.T) {
	t.Run("with no remotes", func(t *testing.T) {
		logger := golog.NewTestLogger(t)
		robot := setupInjectRobot(logger, false, false)

		status, err := status.Create(context.Background(), robot)
		test.That(t, err, test.ShouldBeNil)
		test.That(t, status, test.ShouldResemble, &pb.Status{
			Arms: map[string]*pb.ArmStatus{
				"arm1": {},
				"arm2": {},
			},
			Bases: map[string]bool{
				"base1": true,
				"base2": true,
			},
			Grippers: map[string]bool{
				"gripper1": true,
				"gripper2": true,
			},
			Boards: map[string]*pb.BoardStatus{
				"board1": {},
				"board2": {},
			},
			Cameras: map[string]bool{
				"camera1": true,
				"camera2": true,
			},
			Lidars: map[string]bool{
				"lidar1": true,
				"lidar2": true,
			},
			Sensors: map[string]*pb.SensorStatus{
				"sensor1": {
					Type: "compass",
				},
				"sensor2": {
					Type: "compass",
				},
			},
		})
	})

	t.Run("with remotes", func(t *testing.T) {
		logger := golog.NewTestLogger(t)
		robot := setupInjectRobot(logger, true, true)

		_, err := status.Create(context.Background(), robot)
		test.That(t, err, test.ShouldNotBeNil)
		test.That(t, err.Error(), test.ShouldContainSubstring, "whoops")

		logger = golog.NewTestLogger(t)
		robot = setupInjectRobot(logger, true, false)

		status, err := status.Create(context.Background(), robot)
		test.That(t, err, test.ShouldBeNil)
		// Status is the same as with no remotes because it's up to the
		// robot to utilize information from the remotes. We know
		// Refresh is called due to the failure above.
		test.That(t, status, test.ShouldResemble, &pb.Status{
			Arms: map[string]*pb.ArmStatus{
				"arm1": {},
				"arm2": {},
			},
			Bases: map[string]bool{
				"base1": true,
				"base2": true,
			},
			Grippers: map[string]bool{
				"gripper1": true,
				"gripper2": true,
			},
			Boards: map[string]*pb.BoardStatus{
				"board1": {},
				"board2": {},
			},
			Cameras: map[string]bool{
				"camera1": true,
				"camera2": true,
			},
			Lidars: map[string]bool{
				"lidar1": true,
				"lidar2": true,
			},
			Sensors: map[string]*pb.SensorStatus{
				"sensor1": {
					Type: "compass",
				},
				"sensor2": {
					Type: "compass",
				},
			},
		})
	})
}
