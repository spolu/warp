package out

import (
	"fmt"

	"github.com/fatih/color"
)

var white *color.Color
var bold *color.Color
var cyan *color.Color
var yellow *color.Color
var magenta *color.Color
var redBold *color.Color

func init() {
	white = color.New(color.FgWhite)
	bold = color.New(color.Bold)
	cyan = color.New(color.FgCyan)
	yellow = color.New(color.FgYellow)
	magenta = color.New(color.FgMagenta)
	redBold = color.New(color.FgRed, color.Bold)
}

// Normf prints a normal message.
func Normf(format string, v ...interface{}) {
	fmt.Printf(format, v...)
}

// Boldf prints a bold message.
func Boldf(format string, v ...interface{}) {
	bold.PrintfFunc()(format, v...)
}

// Valuf prints an example message.
func Valuf(format string, v ...interface{}) {
	cyan.PrintfFunc()(format, v...)
}

// Warnf prints a warning message.
func Warnf(format string, v ...interface{}) {
	yellow.PrintfFunc()(format, v...)
}

// Errof prints an error message.
func Errof(format string, v ...interface{}) {
	redBold.PrintfFunc()(format, v...)
}

// Statf prints an error message.
func Statf(format string, v ...interface{}) {
	magenta.PrintfFunc()(format, v...)
}
