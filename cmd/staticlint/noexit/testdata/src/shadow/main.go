package main

import "other"

type exiter struct{}

func (exiter) Exit(int) {}

func main() {
	os := exiter{}
	os.Exit(1)
	Exit := func(int) {}
	Exit(1)
	other.Exit(1)
}
