package main

import (
	"fmt"

	"github.com/akamensky/argparse"
)

func main() {
	// Create new parser object
	parser := argparse.NewParser("helasdfsap", "Demonstrates overriding the default help output")
	// Create string flag
	parser.String("s", "string", &argparse.Options{Help: "String argument example"})
	// Create string flag
	parser.Int("i", "int", &argparse.Options{Required: true, Help: "Integer argument example"})
	// Replace parser.Usage as the help message
	parser.HelpFunc = func(c *argparse.Command, msg interface{}) string {
		var help string
		help += fmt.Sprintf("%s\n", c.GetName())
		help += fmt.Sprintf(" %s\n", c.GetDescription())
		for _, arg := range c.GetArgs() {
			if arg.GetOpts() != nil {
				help += fmt.Sprintf("Sname: %s, Lname: %s, Help: %s\n", arg.GetSname(), arg.GetLname(), arg.GetOpts().Help)
			} else {
				help += fmt.Sprintf("Sname: %s, Lname: %s\n", arg.GetSname(), arg.GetLname())
			}
		}
		return help
	}
	// Use the help function
	err := parser.Parse([]string{"helasdfsap -s", "-i", "3214132"})
	if err != nil {
		// In case of error print error and print usage
		// This can also be done by passing -h or --help flags
		fmt.Print(parser.Usage(err))
	}
	// Finally print the collected string
	// fmt.Println(*s)
}
