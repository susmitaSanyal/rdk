// Package up4000 implements a up4000 based board.
package up4000

import (
	"github.com/edaniels/golog"
	"github.com/pkg/errors"
	"periph.io/x/host/v3"

	"go.viam.com/rdk/components/board/genericlinux"
)

const modelName = "up4000"

func init() {
	if _, err := host.Init(); err != nil {
		golog.Global().Debugw("error initializing host", "error", err)
	}
	gpioMappings, err := genericlinux.GetGPIOBoardMappings(modelName, boardInfoMappings)

	var noBoardErr genericlinux.NoBoardFoundError
	if errors.As(err, &noBoardErr) {
		golog.Global().Debugw("error getting up4000 GPIO board mapping", "error", err)
	}

	genericlinux.RegisterBoard(modelName, gpioMappings, false)
}
