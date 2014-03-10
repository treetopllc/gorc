package main

import (
	"fmt"
	"github.com/stretchr/commander"
	"github.com/stretchr/objx"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// searchTest is the string for searching for test files
	searchTest = "_test.go"

	// searchGo is the string for searching for go files
	searchGo = ".go"
)

func getwd() (string, error) {
	directory, error := os.Getwd()
	if error != nil {
		fmt.Printf(errorCurrentDirectory, error)
	}
	return directory, error
}

func addTimeoutArg(args []string) []string {
	if timeout != "" {
		timeoutOpt := fmt.Sprintf("-timeout=%s", timeout)
		args = append(args, timeoutOpt)
	}
	return args
}

func installTests(name string) bool {
	fmt.Print("\nInstalling tests: ")
	run, failed := runCommand(false, name, searchTest, "go", "test", "-i")
	if run == 0 && failed == 0 {
		fmt.Println("No tests were found in or below the current working directory.")
		return false
	} else {
		fmt.Printf("\n\n%d installed. %d failed. [%.0f%% success]\n\n", run-failed, failed, (float32((run-failed))/float32(run))*100)
	}
	return failed == 0
}

func runTests(name string, verbose bool) bool {
	fmt.Print("Running tests: ")
	run, failed := runCommandParallel(verbose, false, name, searchTest, "go", "test")
	if run == 0 && failed == 0 {
		fmt.Println("No tests were found in or below the current working directory.")
	} else {
		fmt.Printf("\n\n%d run. %d succeeded. %d failed. [%.0f%% success]\n\n", run, run-failed, failed, (float32((run-failed))/float32(run))*100)
	}
	return failed == 0
}

func runCover(name, out, viewer string, coverArgs []string) bool {
	fmt.Print("Generating test coverage: ")
	coverCmd := []string{"test"}
	if out != "" {
		coverCmd = append(coverCmd, "-coverprofile", out)
		coverCmd = addTimeoutArg(coverCmd)
	} else {
		coverCmd = append(coverCmd, "-cover")
		coverCmd = addTimeoutArg(coverCmd)
		coverCmd = append(coverCmd, coverArgs...)
	}
	run, failed := runCommandParallel(false, false, name, searchTest, "go", coverCmd...)
	if run == 0 && failed == 0 {
		fmt.Println("No tests were found in or below the current working directory.")
	} else {
		fmt.Printf("\n\n%d run. %d succeeded. %d failed. [%.0f%% success]\n\n", run, run-failed, failed, (float32((run-failed))/float32(run))*100)
		if failed == 0 && out != "" && viewer != "" {
			viewOpt := fmt.Sprintf("-%s=%s", viewer, out)
			viewArgs := append([]string{"tool", "cover", viewOpt}, coverArgs...)
			runCommandParallel(true, false, name, searchTest, "go", viewArgs...)
		}
	}
	return failed == 0
}

func lintPackages(name string, verbose bool) bool {
	fmt.Printf("\nRunning linter: ")
	run, failed := runCommandParallel(verbose, true, name, searchGo, "golint")
	if run == 0 && failed == 0 {
		fmt.Println("No packages were found in or below the current working directory.")
	} else {
		fmt.Printf("\n\n%d linted. %d succeeded. %d failed. [%.0f%% success]\n\n", run, run-failed, failed, (float32((run-failed))/float32(run))*100)
	}
	return failed == 0
}

func vetPackages(name string, verbose bool) bool {
	fmt.Printf("\nVetting packages: ")
	run, failed := runCommandParallel(verbose, false, name, searchGo, "go", "vet")
	if run == 0 && failed == 0 {
		fmt.Println("No packages were found in or below the current working directory.")
	} else {
		fmt.Printf("\n\n%d vetted. %d succeeded. %d failed. [%.0f%% success]\n\n", run, run-failed, failed, (float32((run-failed))/float32(run))*100)
	}
	return failed == 0
}

func raceTests(name string) {
	fmt.Printf("\nRunning race tests: ")
	run, failed := runCommandParallel(false, false, name, searchTest, "go", "test", "-race")
	if run == 0 && failed == 0 {
		fmt.Println("No tests were found in or below the current working directory.")
	} else {
		fmt.Printf("\n\n%d run. %d succeeded. %d failed. [%.0f%% success]\n\n", run, run-failed, failed, (float32((run-failed))/float32(run))*100)
	}
}

type cmdOutput struct {
	output string
	err    error
}

func countAndPrintOutputs(outputs []cmdOutput, verbose bool) int {
	if len(outputs) != 0 {
		var errCount int
		for _, output := range outputs {
			results := strings.TrimSpace(output.output)
			if results != "" && (verbose || output.err != nil) {
				fmt.Printf("\n\n%s", results)
			}
			if output.err != nil {
				errCount++
			}
		}
		return errCount
	}
	return 0
}

func runCommand(verbose bool, target, search, command string, args ...string) (int, int) {
	var outputs []cmdOutput
	lastPrintLen := 0
	currentJob := 1
	directories := []string{}

	if directory, error := getwd(); error == nil {
		recurseDirectories(directory, target, search,
			func(currentDirectory string) bool {
				if target == "all" {
					return false
				}
				if contains, _ := sliceContainsString(currentDirectory, exclusions); target == "" && contains {
					return true
				}
				return false
			},
			func(currentDirectory string) {
				directories = append(directories, currentDirectory)
			})
	}

	numCommands := len(directories)

	for _, directory := range directories {
		if lastPrintLen == 0 {
			printString := fmt.Sprintf("[%d of %d]", currentJob, numCommands)
			lastPrintLen = len(printString)
			fmt.Print(printString)
		} else {
			printString := fmt.Sprintf("%s[%d of %d]", strings.Repeat("\b", lastPrintLen), currentJob, numCommands)
			lastPrintLen = len(printString) - lastPrintLen
			fmt.Print(printString)
		}

		currentJob++

		output, err := runShellCommand(directory, command, args...)

		outputs = append(outputs, cmdOutput{output, err})
	}

	return currentJob - 1, countAndPrintOutputs(outputs, verbose)
}

func runCommandParallel(verbose, glob bool, target, search, command string, args ...string) (int, int) {
	var outputs []cmdOutput
	lastPrintLen := 0
	currentJob := 1
	directories := []string{}

	if directory, error := getwd(); error == nil {
		recurseDirectories(directory, target, search,
			func(currentDirectory string) bool {
				if target == "all" {
					return false
				}
				if contains, _ := sliceContainsString(currentDirectory, exclusions); target == "" && contains {
					return true
				}
				return false
			},
			func(currentDirectory string) {
				directories = append(directories, currentDirectory)
			})
	}

	numCommands := len(directories)
	outputChan := make(chan cmdOutput, 10)
	var wg sync.WaitGroup
	wg.Add(numCommands)

	for _, directory := range directories {
		go func(dir string) {
			shellArgs := make([]string, len(args))
			copy(shellArgs, args)
			if glob {
				pattern := fmt.Sprintf("%s/*%s", dir, search)
				files, err := filepath.Glob(pattern)
				if err == nil {
					shellArgs = append(shellArgs, files...)
				}
			}
			out, err := runShellCommand(dir, command, shellArgs...)
			outputChan <- cmdOutput{out, err}
		}(directory)
	}

	if lastPrintLen == 0 {
		printString := fmt.Sprintf("[%d of %d]", currentJob, numCommands)
		lastPrintLen = len(printString)
		fmt.Print(printString)
	} else {
		printString := fmt.Sprintf("%s[%d of %d]", strings.Repeat("\b", lastPrintLen), currentJob, numCommands)
		lastPrintLen = len(printString) - lastPrintLen
		fmt.Print(printString)
	}

	go func() {
		for output := range outputChan {

			if lastPrintLen == 0 {
				printString := fmt.Sprintf("[%d of %d]", currentJob, numCommands)
				lastPrintLen = len(printString)
				fmt.Print(printString)
			} else {
				printString := fmt.Sprintf("%s[%d of %d]", strings.Repeat("\b", lastPrintLen), currentJob, numCommands)
				lastPrintLen = len(printString) - lastPrintLen
				fmt.Print(printString)
			}
			currentJob++

			outputs = append(outputs, output)

			wg.Done()
		}
	}()

	wg.Wait()

	return currentJob - 1, countAndPrintOutputs(outputs, verbose)
}

func parseBoolArg(args objx.Map, name string) bool {
	if arg, ok := args[name]; ok {
		argStr := strings.ToLower(arg.(string))
		if argStr != "false" && argStr != "no" {
			return true
		}
	}
	return false
}

var exclusions []string
var timeout string

func main() {

	var config = readConfig()
	exclusions = config[configKeyExclusions].([]string)
	timeout = config[configKeyTimeout].(string)

	commander.Go(func() {
		commander.Map(commander.DefaultCommand, "", "",
			func(args objx.Map) {
				name := ""
				if _, ok := args["name"]; ok {
					name = args["name"].(string)
				}

				success := false
				if installTests(name) {
					success = runTests(name, false)
				} else {
					fmt.Printf("Test dependency installation failed. Aborting test run.\n\n")
				}
				if !success {
					os.Exit(1)
				}
			})

		commander.Map("test [name=(string)] [verbose=(bool)]", "Runs tests, or named test",
			"If no name argument is specified, runs all tests recursively. If a name argument is specified, runs just that test, unless the argument is \"all\", in which case it runs all tests, including those in the exclusion list.",
			func(args objx.Map) {
				name := ""
				if _, ok := args["name"]; ok {
					name = args["name"].(string)
				}
				verbose := parseBoolArg(args, "verbose")

				success := false
				if installTests(name) {
					success = runTests(name, verbose)
				} else {
					fmt.Println("Test dependency installation failed. Aborting test run.")
				}
				if !success {
					os.Exit(1)
				}
			})

		commander.Map("cover [name=(string)] [out=(string)] [viewer=(string)] [coverArgs=(string)...]", "Runs coverage analysis",
			"If an out argument is specified, analysis is saved to the file. A viewer may then be specified in order to display the coverage results. If no name argument is specified, runs all tests recursively. If a name argument is specified, runs just that test, unless the argument is \"all\", in which case it runs all tests, including those in the exclusion list.",
			func(args objx.Map) {
				out := ""
				if arg, ok := args["out"]; ok {
					out = arg.(string)
				}

				name := ""
				if arg, ok := args["name"]; ok {
					name = arg.(string)
				}

				viewer := ""
				if arg, ok := args["viewer"]; ok {
					viewer = arg.(string)
				}

				var coverArgs []string
				if arg, ok := args["coverArgs"]; ok {
					coverArgs = arg.([]string)
				}

				success := false
				if installTests(name) {
					success = runCover(name, out, viewer, coverArgs)
				} else {
					fmt.Println("Test dependency installation failed. Aborting test run.")
				}
				if !success {
					os.Exit(1)
				}
			})

		commander.Map("install [name=(string)]", "Installs tests, or named test",
			"If no name argument is specified, installs all tests recursively. If a name argument is specified, installs just that test, unless the argument is \"all\", in which case it installs all tests, including those in the exclusion list.",
			func(args objx.Map) {
				name := ""
				if _, ok := args["name"]; ok {
					name = args["name"].(string)
				}
				installTests(name)
			})

		commander.Map("lint [name=(string)] [verbose=(bool)]", "Lints packages, or named package",
			"If no name argument is specified, lints all packages recursively. If a name argument is specified, lints just that package, unless the argument is \"all\", in which case it lints all packages, including those in the exclusion list.",
			func(args objx.Map) {
				name := ""
				if _, ok := args["name"]; ok {
					name = args["name"].(string)
				}
				verbose := parseBoolArg(args, "verbose")
				if !lintPackages(name, verbose) {
					os.Exit(1)
				}
			})

		commander.Map("vet [name=(string)] [verbose=(bool)]", "Vets packages, or named package",
			"If no name argument is specified, vets all packages recursively. If a name argument is specified, vets just that package, unless the argument is \"all\", in which case it vets all packages, including those in the exclusion list.",
			func(args objx.Map) {
				name := ""
				if _, ok := args["name"]; ok {
					name = args["name"].(string)
				}
				verbose := parseBoolArg(args, "verbose")
				if !vetPackages(name, verbose) {
					os.Exit(1)
				}
			})

		commander.Map("race [name=(string)]", "Runs race detector on tests, or named test",
			"If no name argument is specified, race tests all tests recursively. If a name argument is specified, vets just that test, unless the argument is \"all\", in which case it vets all tests, including those in the exclusion list.",
			func(args objx.Map) {
				name := ""
				if _, ok := args["name"]; ok {
					name = args["name"].(string)
				}
				raceTests(name)
			})

		commander.Map("exclude name=(string)", "Excludes the named directory from recursion",
			"An excluded directory will be skipped when walking the directory tree. Any subdirectories of the excluded directory will also be skipped.",
			func(args objx.Map) {
				exclude(args["name"].(string), config)
				fmt.Printf("\nExcluded \"%s\" from being examined during recursion.\n", args["name"].(string))
				config = readConfig()
				exclusions = config[configKeyExclusions].([]string)
				fmt.Printf("\n%s\n\n", formatExclusionsForPrint(exclusions))
			})

		commander.Map("include name=(string)", "Removes the named directory from the exclusion list", "",
			func(args objx.Map) {
				include(args["name"].(string), config)
				fmt.Printf("\nRemoved \"%s\" from the exclusion list.\n", args["name"].(string))
				fmt.Printf("\n%s\n\n", formatExclusionsForPrint(exclusions))
			})

		commander.Map("exclusions", "Prints the exclusion list", "",
			func(args objx.Map) {
				fmt.Printf("\n%s\n\n", formatExclusionsForPrint(exclusions))
			})

		commander.Map("timeout value=(string)", "Sets the test timeout", "",
			func(args objx.Map) {
				timeoutAfter(args["value"].(string), config)
				fmt.Printf("\nSet test timeout to \"%s\".\n", args["value"])
			})

	})

}
