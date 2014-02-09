package main

import (
	"fmt"
	"github.com/stretchr/commander"
	"github.com/stretchr/objx"
	"os"
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

func runTests(name string) {
	fmt.Print("Running tests: ")
	run, failed := runCommandParallel(false, name, searchTest, "go", "test")
	if run == 0 && failed == 0 {
		fmt.Println("No tests were found in or below the current working directory.")
	} else {
		fmt.Printf("\n\n%d run. %d succeeded. %d failed. [%.0f%% success]\n\n", run, run-failed, failed, (float32((run-failed))/float32(run))*100)
	}
}

func runCover(name, out, viewer string, coverArgs []string) {
	fmt.Print("Generating test coverage: ")
	coverCmd := []string{"test"}
	if out != "" {
		coverCmd = append(coverCmd, "-coverprofile", out)
	} else {
		coverCmd = append(coverCmd, "-cover")
		coverCmd = append(coverCmd, coverArgs...)
	}
	run, failed := runCommandParallel(false, name, searchTest, "go", coverCmd...)
	if run == 0 && failed == 0 {
		fmt.Println("No tests were found in or below the current working directory.")
	} else {
		fmt.Printf("\n\n%d run. %d succeeded. %d failed. [%.0f%% success]\n\n", run, run-failed, failed, (float32((run-failed))/float32(run))*100)
		if failed == 0 && out != "" && viewer != "" {
			viewOpt := fmt.Sprintf("-%s=%s", viewer, out)
			viewArgs := append([]string{ "tool", "cover", viewOpt }, coverArgs...)
			runCommandParallel(true, name, searchTest, "go", viewArgs...)
		}
	}
}

func vetPackages(name string) {
	fmt.Printf("\nVetting packages: ")
	run, failed := runCommandParallel(false, name, searchGo, "go", "vet")
	if run == 0 && failed == 0 {
		fmt.Println("No packages were found in or below the current working directory.")
	} else {
		fmt.Printf("\n\n%d vetted. %d succeeded. %d failed. [%.0f%% success]\n\n", run, run-failed, failed, (float32((run-failed))/float32(run))*100)
	}
}

func raceTests(name string) {
	fmt.Printf("\nRunning race tests: ")
	run, failed := runCommandParallel(false, name, searchTest, "go", "test", "-race")
	if run == 0 && failed == 0 {
		fmt.Println("No tests were found in or below the current working directory.")
	} else {
		fmt.Printf("\n\n%d run. %d succeeded. %d failed. [%.0f%% success]\n\n", run, run-failed, failed, (float32((run-failed))/float32(run))*100)
	}
}

type cmdOutput struct {
	output string
	err error
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

func runCommandParallel(verbose bool, target, search, command string, args ...string) (int, int) {
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
			out, err := runShellCommand(dir, command, args...)
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

var exclusions []string

func main() {

	var config = readConfig()
	exclusions = config[configKeyExclusions].([]string)

	commander.Go(func() {
		commander.Map(commander.DefaultCommand, "", "",
			func(args objx.Map) {
				name := ""
				if _, ok := args["name"]; ok {
					name = args["name"].(string)
				}

				if installTests(name) {
					runTests(name)
				} else {
					fmt.Printf("Test dependency installation failed. Aborting test run.\n\n")
				}
			})

		commander.Map("test [name=(string)]", "Runs tests, or named test",
			"If no name argument is specified, runs all tests recursively. If a name argument is specified, runs just that test, unless the argument is \"all\", in which case it runs all tests, including those in the exclusion list.",
			func(args objx.Map) {
				name := ""
				if _, ok := args["name"]; ok {
					name = args["name"].(string)
				}

				if installTests(name) {
					runTests(name)
				} else {
					fmt.Println("Test dependency installation failed. Aborting test run.")
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

				if installTests(name) {
					runCover(name, out, viewer, coverArgs)
				} else {
					fmt.Println("Test dependency installation failed. Aborting test run.")
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

		commander.Map("vet [name=(string)]", "Vets packages, or named package",
			"If no name argument is specified, vets all packages recursively. If a name argument is specified, vets just that package, unless the argument is \"all\", in which case it vets all packages, including those in the exclusion list.",
			func(args objx.Map) {
				name := ""
				if _, ok := args["name"]; ok {
					name = args["name"].(string)
				}
				vetPackages(name)
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
	})

}
