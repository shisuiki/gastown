package cmd

import "fmt"

func requireConfirm(confirmed bool, action string) error {
	if confirmed {
		return nil
	}
	if action == "" {
		action = "proceed"
	}
	return fmt.Errorf("refusing to %s without --confirm", action)
}
