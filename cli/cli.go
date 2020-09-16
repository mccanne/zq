package cli

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
)

// Version is set via the Go linker.  See Makefile.
var Version = "unknown"

type Flags struct {
	showVersion    bool
	cpuprofile     string
	memprofile     string
	cpuProfileFile *os.File
}

func (f *Flags) SetFlags(fs *flag.FlagSet) {
	fs.BoolVar(&f.showVersion, "version", false, "print version and exit")
	fs.StringVar(&f.cpuprofile, "cpuprofile", "", "write cpu profile to given file name")
	fs.StringVar(&f.memprofile, "memprofile", "", "write memory profile to given file name")
}

func (f *Flags) Init() (bool, error) {
	if f.cpuprofile != "" {
		f.runCPUProfile(f.cpuprofile)
	}
	if f.showVersion {
		fmt.Printf("Version: %s\n", Version)
		return false, nil
	}
	return true, nil
}

func (f *Flags) Cleanup() {
	if f.cpuProfileFile != nil {
		pprof.StopCPUProfile()
		f.cpuProfileFile.Close()
	}
	if f.memprofile != "" {
		runMemProfile(f.memprofile)
	}
}

func (f *Flags) runCPUProfile(path string) {
	file, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	f.cpuProfileFile = file
	pprof.StartCPUProfile(file)
}

func runMemProfile(path string) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	runtime.GC()
	pprof.Lookup("allocs").WriteTo(f, 0)
	f.Close()
}
