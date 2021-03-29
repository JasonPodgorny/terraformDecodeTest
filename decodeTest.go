// Copyright © 2016 Alan A. A. Donovan & Brian W. Kernighan.
// Copyright © 2020 - Updates By Jason Podgorny.
// License: https://creativecommons.org/licenses/by-nc-sa/4.0/

// The du4 command computes the disk usage of the files in a directory.
// See page 251 in Go Programming Language Book.

// This is a modified version of the du4 program that looks for json and yaml files in a directory

// It still continues to count the files and disk usage of them but in addition it attempts
// To Decode These Files Using The go-cty library which is the same library and functions
// That terraform itself is using for the jsondecode and yamldecode functions.

// The intended purpose of this is as a pre processor for the json and yaml files we are
// Getting from end users inside of terragrunt.

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	ctyyaml "github.com/zclconf/go-cty-yaml"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// Define a type named "stringSlice" as a slice of Strings
type stringSlice []string

func (ss *stringSlice) String() string {
	return strings.Join(*ss, ", ")
}

func (ss *stringSlice) Set(value string) error {
	var stringSlice = strings.Split(value, ",")
	for i := range stringSlice {
		stringSlice[i] = strings.TrimSpace(stringSlice[i])
	}
	*ss = stringSlice
	return nil
}

type SafeCounter struct {
	mu          sync.Mutex
	nbytes      int64
	fileCounts  map[string]int
	errorCounts map[string]int
}

func (sc *SafeCounter) AddBytes(size int64) {
	sc.mu.Lock()
	sc.nbytes += size
	sc.mu.Unlock()
}

func (sc *SafeCounter) AddFile(extension string) {
	sc.mu.Lock()
	sc.fileCounts["total"]++
	sc.fileCounts[extension]++
	sc.mu.Unlock()
}

func (sc *SafeCounter) AddError(extension string) {
	sc.mu.Lock()
	sc.errorCounts["total"]++
	sc.errorCounts[extension]++
	sc.mu.Unlock()
}

// Print Overall file count and usage, YAML file and error count, JSON file and error count
func (sc *SafeCounter) printFileCounts() {
	log.Printf("%d total files  %.1f MB\n", sc.fileCounts["total"], float64(sc.nbytes)/1e6)
	for extension, count := range sc.fileCounts {
		if extension == "total" {
			continue
		}
		log.Printf("%d %s files, %d Decode Errors\n", count, extension, sc.errorCounts[extension])
	}
}

func main() {

	// Set Match Pattern Defaults, And Read From Flags For Overrides
	var matchPatterns = stringSlice{"*.json", "*.yaml"}
	flag.Var(&matchPatterns, "matchpatterns", "List of match patterns")

	// Set ExcludeDir Defaults, And Read From Flags For Overrides
	var excludeDirs = stringSlice{".git", ".terragrunt-cache", "scripts"}
	flag.Var(&excludeDirs, "excludedirs", "List of exclude dirs")

	// Check Flag For Path To Search, Set To Current Directory (.) If None Provided
	pathPtr := flag.String("path", ".", "Path to search")

	flag.Parse()
	extraArgs := flag.Args()

	// If There Are Extra Arguments Beyond Flags, Inputs Were Formatted Improperly
	if len(extraArgs) > 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Initialize Safe Counter
	counter := SafeCounter{
		fileCounts:  map[string]int{"total": 0},
		errorCounts: map[string]int{"total": 0},
	}

	// Create Channels And WaitGroup
	fileSizes := make(chan int64)
	fileNames := make(chan string)
	var n sync.WaitGroup

	// Search Root Recursively
	var roots = []string{*pathPtr}
	for _, root := range roots {
		n.Add(1)
		go walkDir(root, matchPatterns, excludeDirs, &n, fileSizes, fileNames)
	}
	go func() {
		n.Wait()
		close(fileSizes)
		close(fileNames)
	}()

loop:
	for {
		select {
		case size, ok := <-fileSizes:
			if !ok {
				break loop // fileSizes was closed
			}

			// Add to Overall File Size Counter
			counter.AddBytes(size)

		case name, ok := <-fileNames:
			if !ok {
				break loop // fileNames was closed
			}

			// Add File Suffix To File Counter
			fileSuffix := filepath.Ext(name)
			counter.AddFile(fileSuffix)

			decodeSuccess := fileDecode(name)
			if !decodeSuccess {
				// Add File Suffix To Error Counter
				counter.AddError(fileSuffix)
			}

		}
	}
	counter.printFileCounts() // final totals

	// See If There Were Errors Decoding Any Files
	// If No Errors, Log All Successful And Exit 0
	// If Errors, Indicate Failure and Exit 1
	if counter.errorCounts["total"] > 0 {
		log.Fatalf("Decode Errors Found In Files")
	} else {
		log.Printf("All Files Decoded Successfully")
	}
}

// walkDir recursively walks the file tree rooted at dir
// and sends the size of each found file on fileSizes.
func walkDir(dir string, matchPatterns stringSlice, excludeDirs stringSlice, n *sync.WaitGroup, fileSizes chan<- int64, fileNames chan<- string) {
	defer n.Done()

	for _, entry := range dirents(dir) {
		// If Entry Is Directory And Not In excludedDirs Recursively Walk It
		if entry.IsDir() && contains(excludeDirs, entry.Name()) == false {
			n.Add(1)
			subdir := filepath.Join(dir, entry.Name())
			go walkDir(subdir, matchPatterns, excludeDirs, n, fileSizes, fileNames)
		} else {
			// If Entry Is Not A Directory, Test For Pattern Match.   Exclude Files with Size 0
			// Those don't need to be decoded.
			for _, pattern := range matchPatterns {
				if match, _ := filepath.Match(pattern, entry.Name()); match == true && entry.Size() > 0 {
					fileSizes <- entry.Size()
					fileNames <- filepath.Join(dir, entry.Name())
				}
			}
		}
	}
}

var sema = make(chan struct{}, 20) // concurrency-limiting counting semaphore

// dirents returns the entries of directory dir.
func dirents(dir string) []os.FileInfo {

	sema <- struct{}{}        // acquire token
	defer func() { <-sema }() // release token

	f, err := os.Open(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decodeTest: %v\n", err)
		return nil
	}
	defer f.Close()

	entries, err := f.Readdir(0) // 0 => no limit; read all entries
	if err != nil {
		fmt.Fprintf(os.Stderr, "decodeTest: %v\n", err)
		// Don't return: Readdir may return partial results.
	}
	return entries
}

func fileDecode(filename string) bool {

	sema <- struct{}{}        // acquire token
	defer func() { <-sema }() // release token

	var decodeFuncs = map[string]function.Function{
		".yaml": ctyyaml.YAMLDecodeFunc,
		".json": stdlib.JSONDecodeFunc,
	}

	fileSuffix := filepath.Ext(filename)
	decodeFunction, ok := decodeFuncs[fileSuffix]
	if !ok {
		log.Printf("No Decoder For File Type %s: %s", fileSuffix, filename)
		return false
	}

	fileString, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Printf("error reading file %s: %v", filename, err)
		return false
	}

	ctyValues := []cty.Value{
		cty.StringVal(string(fileString)),
	}

	_, err = decodeFunction.Call(ctyValues)
	if err != nil {
		log.Printf("error decoding file %s: %v", filename, err)
		return false
	}

	return true

}

func contains(slice []string, item string) bool {
	set := make(map[string]struct{}, len(slice))
	for _, s := range slice {
		set[s] = struct{}{}
	}

	_, ok := set[item]
	return ok
}
