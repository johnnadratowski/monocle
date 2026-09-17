package main

import "fmt"

// relaunchSelf has no Windows equivalent: there is no exec that replaces the
// running process, and starting a child while this one still owns the console
// would leave two processes fighting over it. The reviewer is told to restart
// instead, which is what they would have done anyway.
func relaunchSelf() error {
	return fmt.Errorf("relaunch is not supported on Windows — start monocle again to pick up the new build")
}
