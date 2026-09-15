// Command gnd is the game numerical design CLI.
package main

import (
	"os"

	"github.com/neko233-com/game-numerical-design-cli/internal/cli"
)

func main() {
	app := &cli.App{Args: os.Args[1:]}
	os.Exit(app.Run())
}
