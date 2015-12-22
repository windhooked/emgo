// stm32xgen generates STM32 peripheral filex in xgen format.
//
// stm32xgen is usually used this wahy:
//  unifdef -k -f undef.h -D STM32TARGET stm32f4xx.h |stm32xgen PKGPATH
package main

import (
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 2 {
		die("Usage: %s PKGPATH")
	}
	pkgpath := os.Args[1]
	base := filepath.Base(pkgpath)
	mkdir(base)
	chdir(base)
	var (
		pkgs []*Package
		mmap []*MemGroup
	)
	r := newScanner(os.Stdin, "stdin")
	for r.Scan() {
	noscan:
		switch doxy(r.Text(), "@addtogroup") {
		case "Peripheral_registers_structures":
			pkgs = peripherals(r)
		case "Peripheral_memory_map":
			mmap = memmap(r)
		case "Peripheral_declaration":
			instances(r, pkgs)
		case "Peripheral_Registers_Bits_Definition":
			bits(r, pkgs)
		default:
			continue
		}
		goto noscan
	}
	checkErr(r.Err())
	saveMmap(mmap)
	for _, pkg := range pkgs {
		pkg.Save(pkgpath)
	}

}
