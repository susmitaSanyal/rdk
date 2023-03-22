// Package up4000 implements a up4000 based board.
package up4000

import (
	"github.com/edaniels/golog"
	"github.com/pkg/errors"
	"periph.io/x/host/v3"

	"go.viam.com/rdk/components/board/genericlinux"
)

const modelName = "up40000"

func init() {
	golog.Global().Info("in the init function")
	if _, err := host.Init(); err != nil {
		golog.Global().Debugw("error initializing host", "error", err)
	}
	golog.Global().Info("Initialized host")
	golog.Global().Info("now mapping gpio")
	gpioMappings, err := genericlinux.GetGPIOBoardMappings(modelName, boardInfoMappings)
	golog.Global().Info("finished mapping gpio")

	var noBoardErr genericlinux.NoBoardFoundError
	if errors.As(err, &noBoardErr) {
		golog.Global().Debugw("error getting up4000 GPIO board mapping", "error", err)
	}
	golog.Global().Info("no errors yet")
	golog.Global().Info("registering board")
	genericlinux.RegisterBoard(modelName, gpioMappings, true)
}
