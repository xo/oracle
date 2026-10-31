package repotest

import (
	"fmt"
	"os"
	"testing"
)

// TestMain moves to the repository root, where the files these tests read are.
func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
