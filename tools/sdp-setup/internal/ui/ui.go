// Package ui provides interactive user interface utilities for the setup wizard
package ui

import (
	"errors"
	"fmt"
	"os"

	"github.com/manifoldco/promptui"
)

type ConfirmationOption string

const (
	ConfirmationDefaultNo  ConfirmationOption = "N"
	ConfirmationDefaultYes ConfirmationOption = "y"

	SIGINT = 130 // Standard exit code for SIGINT
)

// Confirm prompts the user for a yes/no confirmation with default as No
// Returns true if the user confirms (y/Y), false otherwise or on error
func Confirm(label string) bool {
	return ConfirmWithDefault(label, ConfirmationDefaultNo)
}

// ConfirmWithDefault prompts the user for a yes/no confirmation with specified default
func ConfirmWithDefault(label string, option ConfirmationOption) bool {
	prompt := promptui.Prompt{
		Label:     label,
		IsConfirm: true,
	}

	defaultYes := option == ConfirmationDefaultYes

	if defaultYes {
		prompt.Default = "y"
	}

	res, err := prompt.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrInterrupt) {
			fmt.Println("\nSetup cancelled.")
			os.Exit(SIGINT)
		}
		return false
	}

	if res == "" {
		return defaultYes
	}

	return res == "y" || res == "Y"
}

// Select presents a list of options for the user to choose from
// Returns the selected item or empty string on error
func Select(label string, items []string) string {
	if len(items) == 0 {
		return ""
	}

	sel := promptui.Select{
		Label: label,
		Items: items,
	}

	_, result, err := sel.Run()
	if err != nil {
		if errors.Is(err, promptui.ErrInterrupt) {
			fmt.Println("\nSetup cancelled.")
			os.Exit(SIGINT)
		}
		return ""
	}

	return result
}

// Input prompts the user for text input with optional validation
// Continues prompting until valid input is provided or user cancels
func Input(label string, validate func(string) error) string {
	prompt := promptui.Prompt{
		Label:    label,
		Validate: validate,
	}

	for {
		res, err := prompt.Run()
		if err != nil {
			if errors.Is(err, promptui.ErrInterrupt) {
				fmt.Println("\nSetup cancelled.")
				os.Exit(SIGINT)
			}
			return ""
		}

		if validate == nil {
			return res
		}

		if err = validate(res); err == nil {
			return res
		}

		fmt.Println("❌ Invalid value, please try again.")
	}
}
