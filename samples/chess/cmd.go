// Package main is a chess game featuring a robot versus a human.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	"math"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"sync/atomic"
	"time"

	"go.uber.org/multierr"

	"go.viam.com/core/arm"
	"go.viam.com/core/artifact"
	"go.viam.com/core/config"
	"go.viam.com/core/gripper"
	pb "go.viam.com/core/proto/api/v1"
	"go.viam.com/core/rimage"
	"go.viam.com/core/rimage/imagesource"
	"go.viam.com/core/robot"
	builtinrobot "go.viam.com/core/robot/builtin"
	"go.viam.com/core/utils"
	"go.viam.com/core/vision/chess"
	"go.viam.com/core/web"

	"github.com/edaniels/golog"
	"github.com/edaniels/gostream"
	"github.com/tonyOreglia/glee/pkg/engine"
	"github.com/tonyOreglia/glee/pkg/moves"
	"github.com/tonyOreglia/glee/pkg/position"
)

type pos struct {
	x int64
	y int64
}

var (
	BoardWidth     = int64(381)
	Center         = pos{-435, 0}
	BoardHeight    = int64(-230)
	SafeMoveHeight = BoardHeight + 150

	wantPicture = int32(0)

	numPiecesCaptured = 0
	logger            = golog.NewDevelopmentLogger("chess")
)

func getCoord(chess string) pos {
	var x = float64(chess[0] - 'a')
	var y = float64(chess[1] - '1')

	if x < 0 || x > 7 || y < 0 || y > 7 {
		panic(fmt.Errorf("invalid position: %s", chess))
	}

	x = (3.5 - x) / 7.0
	y = (3.5 - y) / 7.0

	return pos{Center.x + int64((x * float64(BoardWidth))), Center.y + int64((y * float64(BoardWidth)))} // HARD CODED
}

func moveTo(ctx context.Context, myArm arm.Arm, chess string, heightModMillis int64) error {
	// first make sure in safe position
	where, err := myArm.CurrentPosition(ctx)
	if err != nil {
		return err
	}
	where.Z = SafeMoveHeight + heightModMillis
	err = myArm.MoveToPosition(ctx, where)
	if err != nil {
		return err
	}

	// move
	if chess == "-" {
		f := getCoord("a8")
		where.X = f.x - int64(60*numPiecesCaptured) // HARD CODED
		where.Y = f.y - (BoardWidth / 5)            // HARD CODED
		numPiecesCaptured = numPiecesCaptured + 1
	} else {
		f := getCoord(chess)
		where.X = f.x
		where.Y = f.y
	}
	return myArm.MoveToPosition(ctx, where)
}

func movePiece(ctx context.Context, boardState boardStateGuesser, robot robot.Robot, myArm arm.Arm, myGripper gripper.Gripper, from, to string) error {

	if to[0] != '-' {
		toHeight, err := boardState.game.GetPieceHeight(boardState.NewestBoard(), to)
		if err != nil {
			return err
		}
		if toHeight > 0 {
			logger.Debugf("moving piece from %s to %s but occupied, going to capture", from, to)
			err = movePiece(ctx, boardState, robot, myArm, myGripper, to, "-")
			if err != nil {
				return err
			}
		}
	}

	err := moveTo(ctx, myArm, from, 0)
	if err != nil {
		return err
	}

	// open before going down
	err = myGripper.Open(ctx)
	if err != nil {
		return err
	}

	err = adjustArmInsideSquare(context.Background(), robot)
	if err != nil {
		return err
	}

	height := boardState.NewestBoard().SquareCenterHeight(from, 35) // TODO(erh): change to something more intelligent
	where, err := myArm.CurrentPosition(ctx)
	if err != nil {
		return err
	}
	where.Z = BoardHeight + int64(height) + int64(10)
	myArm.MoveToPosition(ctx, where)

	// grab piece
	for {
		grabbedSomething, err := myGripper.Grab(ctx)
		if err != nil {
			return err
		}
		if grabbedSomething {
			logger.Debugf("got a piece at height %f", where.Z)
			// got the piece
			break
		}
		err = myGripper.Open(ctx)
		if err != nil {
			return err
		}
		logger.Debug("no piece")
		where, err = myArm.CurrentPosition(ctx)
		if err != nil {
			return err
		}
		where.Z = where.Z - 10
		if where.Z <= BoardHeight {
			return errors.New("no piece")
		}
		myArm.MoveToPosition(ctx, where)
	}

	saveZ := where.Z // save the height to bring the piece down to

	if to == "-throw" {

		err = moveOutOfWay(ctx, myArm)
		if err != nil {
			return err
		}

		utils.PanicCapturingGo(func() {
			if !utils.SelectContextOrWait(ctx, 200*time.Millisecond) {
				return
			}
			myGripper.Open(ctx)
		})
		err = myArm.JointMoveDelta(ctx, 4, -1)
		if err != nil {
			return err
		}

		return initArm(ctx, myArm) // this is to get joint position right
	}

	err = moveTo(ctx, myArm, to, 100)
	if err != nil {
		return err
	}

	// drop piece
	where, err = myArm.CurrentPosition(ctx)
	if err != nil {
		return err
	}

	where.Z = saveZ
	myArm.MoveToPosition(ctx, where)

	myGripper.Open(ctx)

	if to != "-" {
		where, err = myArm.CurrentPosition(ctx)
		if err != nil {
			return err
		}
		where.Z = SafeMoveHeight
		myArm.MoveToPosition(ctx, where)

		moveOutOfWay(ctx, myArm)
	}
	return nil
}

func moveOutOfWay(ctx context.Context, myArm arm.Arm) error {
	foo := getCoord("a1")

	where, err := myArm.CurrentPosition(ctx)
	if err != nil {
		return err
	}
	where.X = foo.x
	where.Y = foo.y
	where.Z = SafeMoveHeight + 300 // HARD CODED

	return myArm.MoveToPosition(ctx, where)
}

func initArm(ctx context.Context, myArm arm.Arm) error {
	foo := getCoord("a1")
	err := myArm.MoveToPosition(ctx, &pb.ArmPosition{
		X:  foo.x,
		Y:  foo.y,
		Z:  SafeMoveHeight,
		RX: -180,
		RY: 0,
		RZ: 0,
	})

	if err != nil {
		return err
	}

	return moveOutOfWay(ctx, myArm)
}

func searchForNextMove(p *position.Position) (*position.Position, *moves.Move) {
	//mvs := generate.GenerateMoves(p)
	perft := 0
	singlePlyPerft := 0
	params := engine.SearchParams{
		Depth:          3,
		Ply:            3,
		Pos:            &p,
		Perft:          &perft,
		SinglePlyPerft: &singlePlyPerft,
		EngineMove:     &moves.Move{},
	}
	if p.IsWhitesTurn() {
		engine.AlphaBetaMax(-10000, 10000, params.Ply, params)
	} else {
		engine.AlphaBetaMin(-10000, 10000, params.Ply, params)
	}
	return p, params.EngineMove
}

func getWristPicCorners(ctx context.Context, wristCam gostream.ImageSource, debugNumber int) ([]image.Point, image.Point, error) {
	imageSize := image.Point{}
	img, release, err := wristCam.Next(ctx)
	if err != nil {
		return nil, imageSize, err
	}
	defer release()
	imgBounds := img.Bounds()
	imageSize.X = imgBounds.Max.X
	imageSize.Y = imgBounds.Max.Y

	// wait, cause this camera sucks
	if !utils.SelectContextOrWait(ctx, 500*time.Millisecond) {
		return nil, imageSize, ctx.Err()
	}
	img, release, err = wristCam.Next(ctx)
	if err != nil {
		return nil, imageSize, err
	}
	defer release()

	// got picture finally
	out, corners, err := chess.FindChessCornersPinkCheat(rimage.ConvertToImageWithDepth(img), logger)
	if err != nil {
		return nil, imageSize, err
	}

	if debugNumber >= 0 {
		if err := rimage.WriteImageToFile(fmt.Sprintf("/tmp/foo-%d-in.png", debugNumber), img); err != nil {
			panic(err)
		}
		if err := rimage.WriteImageToFile(fmt.Sprintf("/tmp/foo-%d-out.png", debugNumber), out); err != nil {
			panic(err)
		}
	}

	logger.Debugf("Corners: %v", corners)

	return corners, imageSize, err
}

func lookForBoardAdjust(ctx context.Context, myArm arm.Arm, wristCam gostream.ImageSource, corners []image.Point, imageSize image.Point) error {
	debugNumber := 100
	for {
		where, err := myArm.CurrentPosition(ctx)
		if err != nil {
			return err
		}
		center := rimage.Center(corners, 10000)

		xRatio := float64(center.X) / float64(imageSize.X)
		yRatio := float64(center.Y) / float64(imageSize.Y)

		xMove := (.5 - xRatio) / 8
		yMove := (.5 - yRatio) / -8

		logger.Debugf("center %v xRatio: %1.4v yRatio: %1.4v xMove: %1.4v yMove: %1.4f", center, xRatio, yRatio, xMove, yMove)

		if math.Abs(xMove) < .001 && math.Abs(yMove) < .001 {
			Center = pos{where.X, where.Y}

			// These are hard coded based on camera orientation
			Center.x += 26
			Center.y -= 73

			logger.Debugf("Center: %v", Center)
			logger.Debugf("a1: %v", getCoord("a1"))
			logger.Debugf("h8: %v", getCoord("h8"))
			return nil
		}

		where.X += int64(xMove * 1000)
		where.Y += int64(yMove * 1000)
		err = myArm.MoveToPosition(ctx, where)
		if err != nil {
			return err
		}

		corners, _, err = getWristPicCorners(ctx, wristCam, debugNumber)
		debugNumber = debugNumber + 1
		if err != nil {
			return err
		}
	}

}

func lookForBoard(ctx context.Context, myArm arm.Arm, myRobot robot.Robot) error {
	debugNumber := 0

	wristCam := myRobot.CameraByName("wristCam")
	if wristCam == nil {
		return errors.New("can't find wristCam")
	}

	for foo := -1.0; foo <= 1.0; foo += 2 {
		// HARD CODED
		where, err := myArm.CurrentPosition(ctx)
		if err != nil {
			return err
		}
		where.X = -420
		where.Y = 20
		where.Z = 600
		where.RX = -2.600206
		where.RY = -0.007839
		where.RZ = -0.061827
		err = myArm.MoveToPosition(ctx, where)
		if err != nil {
			return err
		}

		d := .1
		for i := 0.0; i < 1.6; i = i + d {
			err = myArm.JointMoveDelta(ctx, 0, foo*d)
			if err != nil {
				return err
			}

			corners, imageSize, err := getWristPicCorners(ctx, wristCam, debugNumber)
			debugNumber = debugNumber + 1
			if err != nil {
				return err
			}

			if len(corners) == 4 {
				return lookForBoardAdjust(ctx, myArm, wristCam, corners, imageSize)
			}
		}
	}

	return nil

}

func adjustArmInsideSquare(ctx context.Context, robot robot.Robot) error {
	// wait for camera to focus
	if !utils.SelectContextOrWait(ctx, 500*time.Millisecond) {
		return ctx.Err()
	}

	cam := robot.CameraByName("gripperCam")
	if cam == nil {
		return errors.New("can't find gripperCam")
	}

	arm := robot.ArmByName("pieceArm")

	for {
		where, err := arm.CurrentPosition(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("starting at: %v,%v\n", where.X, where.Y)

		raw, release, err := cam.Next(ctx)
		if err != nil {
			return err
		}
		var dm *rimage.DepthMap
		func() {
			defer release()
			dm = rimage.ConvertToImageWithDepth(raw).Depth
		}()
		if dm == nil {
			return errors.New("no depth on gripperCam")
		}
		//defer img.Close() // TODO(erh): fix the leak
		fmt.Println("\t got image")

		center := image.Point{dm.Width() / 2, dm.Height() / 2}
		lowest, lowestValue, _, highestValue := findDepthPeaks(dm, center, 30)

		diff := highestValue - lowestValue

		if diff < 11 {
			return fmt.Errorf("no chess piece because height is only: %v", diff)
		}

		offsetX := center.X - lowest.X
		offsetY := center.Y - lowest.Y

		if utils.AbsInt(offsetX) < 3 && utils.AbsInt(offsetY) < 3 {
			fmt.Println("success!")
			return nil
		}

		fmt.Printf("\t offsetX: %v offsetY: %v diff: %v\n", offsetX, offsetY, diff)

		where.X += int64(offsetX / -2)
		where.Y += int64(offsetY / 2)

		fmt.Printf("\t moving to %v,%v\n", where.X, where.Y)

		err = arm.MoveToPosition(ctx, where)
		if err != nil {
			return err
		}

		// wait for camera to focus
		if !utils.SelectContextOrWait(ctx, 500*time.Millisecond) {
			return ctx.Err()
		}
	}

}

func main() {
	utils.ContextualMain(mainWithArgs, logger)
}

func mainWithArgs(ctx context.Context, args []string, logger golog.Logger) (err error) {
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")

	flag.Parse()

	cfgFile := flag.Arg(0)

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			return err
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	cfg, err := config.Read(cfgFile)
	if err != nil {
		return err
	}

	myRobot, err := builtinrobot.NewRobot(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		err = multierr.Combine(myRobot.Close())
	}()

	myArm := myRobot.ArmByName("pieceArm")
	if myArm == nil {
		return errors.New("need an arm called pieceArm")
	}

	myGripper := myRobot.GripperByName("grippie")
	if myGripper == nil {
		return errors.New("need a gripper called gripped")
	}

	webcam := myRobot.CameraByName("cameraOver")
	if webcam == nil {
		return errors.New("can't find cameraOver camera")
	}

	if false { // TODO(erh): put this back once we have a wrist camera again
		err = lookForBoard(ctx, myArm, myRobot)
		if err != nil {
			return err
		}
	}

	err = initArm(ctx, myArm)
	if err != nil {
		return err
	}

	if false {
		fmt.Println("ELIOT HACK")

		err = moveTo(ctx, myArm, "c3", 0)
		if err == nil {
			// wait for camera to focus
			if !utils.SelectContextOrWait(ctx, 500*time.Millisecond) {
				return
			}
			err = adjustArmInsideSquare(ctx, myRobot)
		}

		return
	}

	boardState := boardStateGuesser{}
	defer boardState.Clear()
	currentPosition := position.StartingPosition()

	initialPositionOk := false

	annotatedImageHolder := &imagesource.StaticSource{}
	myRobot.AddCamera(annotatedImageHolder, config.Component{})

	utils.PanicCapturingGo(func() {
		for {
			img, release, err := webcam.Next(ctx)
			func() {
				defer release()
				if err != nil {
					logger.Debugf("error reading device: %s", err)
					return
				}

				theBoard, err := chess.FindAndWarpBoard(rimage.ConvertToImageWithDepth(img), logger)
				if err != nil {
					logger.Debug(err)
					return
				}

				annotated := theBoard.Annotate()

				if theBoard.IsBoardBlocked() {
					logger.Debug("board blocked")
					boardState.Clear()
					wantPicture = 1
				} else {
					// boardState now owns theBoard
					interessting, err := boardState.newData(theBoard)
					if err != nil {
						wantPicture = 1
						logger.Debug(err)
						boardState.Clear()
					} else if interessting {
						wantPicture = 1
					}
					theBoard = nil // indicate theBoard is no longer owned

					if boardState.Ready() {
						if !initialPositionOk {
							bb, err := boardState.GetBitBoard()
							if err != nil {
								logger.Debug("got inconsistency reading board, let's try again")
								boardState.Clear()
							} else if currentPosition.AllOccupiedSqsBb().Value() != bb.Value() {
								logger.Debug("not in initial chess piece setup")
								bb.Print()
							} else {
								initialPositionOk = true
								logger.Debug("GOT initial chess piece setup")
							}
						} else {
							// so we've already made sure we're safe, let's see if a move was made
							m, err := boardState.GetPrevMove(currentPosition)
							if err != nil {
								// trouble reading board, let's reset
								logger.Debug("got inconsistency reading board, let's try again")
								boardState.Clear()
							} else if m != nil {
								logger.Debugf("we detected a move: %s", m)

								if !engine.MakeValidMove(*m, &currentPosition) {
									panic("invalid move!")
								}

								currentPosition.Print()
								currentPosition.PrintFen()

								currentPosition, m = searchForNextMove(currentPosition)
								logger.Debugf("computer will make move: %s", m)
								err = movePiece(ctx, boardState, myRobot, myArm, myGripper, m.String()[0:2], m.String()[2:])
								if err != nil {
									panic(err)
								}
								if !engine.MakeValidMove(*m, &currentPosition) {
									panic("wtf - invalid move chosen by computer")
								}
								currentPosition.Print()
								boardState.Clear()
							}
						}

					}
				}

				annotatedImageHolder.Img = rimage.ConvertToImageWithDepth(annotated)
				if atomic.LoadInt32(&wantPicture) != 0 {
					tm := time.Now().Unix()

					fn := artifact.MustNewPath(fmt.Sprintf("samples/chess/board-%d.both.gz", tm))
					logger.Debugf("saving image %s", fn)
					if err := annotatedImageHolder.Img.WriteTo(fn); err != nil {
						panic(err)
					}

					atomic.StoreInt32(&wantPicture, 0)
				}
			}()
		}
	})

	return web.RunWeb(ctx, myRobot, web.NewOptions(), logger)
}
