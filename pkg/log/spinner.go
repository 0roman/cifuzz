package log

import (
	"github.com/pterm/pterm"
)

const (
	BuildInProgressMsg        string = "Build in progress..."
	BuildInProgressSuccessMsg string = "Build in progress... Done."
	BuildInProgressErrorMsg   string = "Build in progress... Error."

	BundleInProgressMsg        string = "Bundle in progress..."
	BundleInProgressSuccessMsg string = "Bundle in progress... Done."
	BundleInProgressErrorMsg   string = "Bundle in progress... Error."
)

func GetPtermErrorStyle() *pterm.Style {
	return &pterm.Style{pterm.FgRed, pterm.Bold}
}

func GetPtermSuccessStyle() *pterm.Style {
	return &pterm.Style{pterm.FgGreen}
}

// Set this, so it can be checked and used in the logging process
// to ensure correct output
var currentProgressSpinner *pterm.SpinnerPrinter

func CreateCurrentProgressSpinner(style *pterm.Style, msg string) {
	// error can be ignored here since pterm doesn't return one
	if style != nil {
		currentProgressSpinner.Style = style
		currentProgressSpinner.MessageStyle = style
	}
	currentProgressSpinner, _ = pterm.DefaultSpinner.Start(msg)
}

func StopCurrentProgressSpinner(style *pterm.Style, msg string) {
	if style != nil {
		currentProgressSpinner.Style = style
		currentProgressSpinner.MessageStyle = style
	}

	if msg != "" {
		currentProgressSpinner.UpdateText(msg)
	}

	// error can be ignored here since pterm doesn't return one
	currentProgressSpinner.RemoveWhenDone = false
	_ = currentProgressSpinner.Stop()
	currentProgressSpinner = nil
}
