package up4000

import "go.viam.com/rdk/components/board/genericlinux"

const bbAi = "bb_Ai64"

var boardInfoMappings = map[string]genericlinux.BoardInformation{
	bbAi: {
		PinDefinitions: []genericlinux.PinDefinition{
			// GPIO pin definition
			{map[int]int{28: 5}, map[int]string{}, "600000.gpio", 29, 0, "GPIO10", "", "", -1},
			{map[int]int{28: 6}, map[int]string{}, "600000.gpio", 31, 0, "GPIO11", "", "", -1},
			{map[int]int{28: 14}, map[int]string{}, "600000.gpio", 8, 0, "GPIO15", "", "", -1},
			{map[int]int{28: 15}, map[int]string{}, "600000.gpio", 10, 0, "GPIO16", "", "", -1},
			{map[int]int{28: 16}, map[int]string{}, "600000.gpio", 36, 0, "GPIO25", "", "", -1},
			{map[int]int{28: 23}, map[int]string{}, "600000.gpio", 16, 0, "GPIO18", "", "", -1},
			{map[int]int{28: 24}, map[int]string{}, "600000.gpio", 18, 0, "GPIO19", "", "", -1},
			{map[int]int{28: 25}, map[int]string{}, "600000.gpio", 22, 0, "GPIO20", "", "", -1},
			{map[int]int{28: 26}, map[int]string{}, "600000.gpio", 37, 0, "GPIO14", "", "", -1},
			{map[int]int{28: 27}, map[int]string{}, "600000.gpio", 13, 0, "GPIO4", "", "", -1},
		},
		Compats: []string{"up4000, UP-APL03P4F-A10-0864"},
	},
}
