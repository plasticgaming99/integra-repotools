package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archives"
)

func main() {
	startedDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	var (
		reponame string
		packs    []string
		cpAll    bool
	)

	for _, st := range os.Args[1:] {
		if strings.HasPrefix(st, "-") {
			switch st {
			case "-h":
				puthelp()
				return
			case "--create-repo":
				fmt.Print("repo name: ")
				reponame = readline()
				fmt.Println("create repo to ./root/", reponame)
				cpAll = true
			}
		} else {
			packs = append(packs, st)
		}
	}

	archvname := "intg.db.tar.zst"

	if len(packs) == 0 {
		fmt.Println("Specify packages to register")
		puthelp()
		return
	}

	_, err = os.Stat(archvname)
	if err != nil {
		f, err := os.OpenFile(archvname, os.O_CREATE, os.ModePerm)
		if err != nil {
			fmt.Println("Error during write db")
			panic(err)
		}
		f.Close()
	}
	archiv, err := os.OpenFile(archvname, os.O_RDWR|os.O_APPEND, os.ModePerm)
	if err != nil {
		fmt.Println("Error during opening db")
		panic(err)
	}
	defer archiv.Close()

	for _, s := range packs {
		abs, err := filepath.Abs(s)
		if err != nil {
			panic(err)
		}
		fmt.Println("Try to add ", abs)

		fsys := archives.ArchiveFS{
			Path:   abs,
			Format: archives.Tar{},
		}
		errorread := func() { fmt.Println("Error during reading archive") }

		var f fs.File
		f, err = fsys.Open(".PACKAGE")
		if err != nil {
			errorread()
		}

		var packagename string

		buff := new(bytes.Buffer)
		buff.Grow(10000)
		read := io.TeeReader(f, buff)
		rescan := bufio.NewScanner(read)
		for rescan.Scan() {
			if strings.HasPrefix(rescan.Text(), "package") {
				packagename = strings.Split(rescan.Text(), " = ")[1]
			}
		}
		initdbdir("dbcache")
		os.Chdir("dbcache")
		initdbdir(packagename)
		os.Chdir(packagename)
		wrtr, err := os.OpenFile(".PACKAGE", os.O_CREATE|os.O_RDWR, os.ModePerm)
		if err != nil {
			panic(err)
		}
		_, err = io.Copy(wrtr, buff)
		if err != nil {
			fmt.Println("Error writing dbcache")
		}
		buff.Reset()
		f.Close()

		f, err = fsys.Open(".MTREE")
		if err != nil {
			errorread()
			panic(err)
		}
		writ, err := os.OpenFile(".MTREE", os.O_CREATE|os.O_RDWR, os.ModePerm)
		if err != nil {
			errorread()
			panic(err)
		}
		_, err = io.Copy(writ, f)
		if err != nil {
			fmt.Println("Error writing dbcache")
		}
		f.Close()
		os.Chdir("../../")
	}

	os.Chdir(startedDir)

	arc := archives.CompressedArchive{
		Archival:    archives.Tar{},
		Compression: archives.Zstd{},
	}

	dbfile, err := os.Create("intg.db.tar.zst")
	if err != nil {
		fmt.Println("Error during creating archive")
		panic(err)
	}

	filemap := make(map[string]string)
	err = filepath.WalkDir("dbcache", func(path string, info fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		realpath := filepath.Join(startedDir, path)
		arcpath := strings.ReplaceAll(path, "dbcache/", "")
		filemap[realpath] = arcpath
		return nil
	})
	if err != nil {
		fmt.Println("Error during scanning dbcache directory")
		panic(err)
	}

	files, err := archives.FilesFromDisk(context.Background(), nil, filemap)
	if err != nil {
		fmt.Println("Error during creating archive")
		panic(err)
	}

	arc.Archive(context.Background(), dbfile, files)

	fmt.Println("Success")

	if !cpAll {
		os.Exit(0)
	}

	errcp := func(eror error) {
		fmt.Println("error during copying archive files")
		fmt.Println("try to copy manually")
		log.Fatal(eror)
	}

	for _, s := range packs {
		wd, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		err = os.MkdirAll(filepath.Join("root", reponame), os.ModePerm)
		if err != nil {
			panic(err)
		}
		//correctS := filepath.Dir(s)
		packname := filepath.Base(s)
		packpath := filepath.Join(wd, "root", reponame, packname)
		dst, err := os.OpenFile(packpath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
		if err != nil {
			errcp(err)
		}
		defer dst.Close()

		file, err := os.Open(s)
		if err != nil {
			errcp(err)
		}

		_, err = io.Copy(dst, file)
		if err != nil {
			errcp(err)
		}

		intgdst, err := os.Create(filepath.Join(wd, "root", reponame, reponame+".db.tar.zst"))
		if err != nil {
			errcp(err)
		}
		arcfile, err := os.Open(filepath.Join(wd, "intg.db.tar.zst"))
		if err != nil {
			errcp(err)
		}
		_, err = io.Copy(intgdst, arcfile)
		if err != nil {
			errcp(err)
		}

	}
}

func puthelp() {
	/*fmtnl := func(st ...string) string {
		var sb strings.Builder
		sb.Grow(len(st) * 2)
		for _, st := range st {
			sb.WriteString(st)
			sb.WriteString("\n")
		}
		return sb.String()
	}*/

	fmt.Println(`REPOTOOL USAGE
  repotool [option] (packages...)
  OPTIONS
    -h            SHOW THIS HELP
    --init        INITIALIZE REPO
    --create-repo CREATE REPO TO ./root DIRECTORY
                  SHOULD USE SYMLINK`)
}

func readline() string {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	return scanner.Text()
}

func initdbdir(path string) error {
	inf, err := os.Stat(path)
	if err != nil {
		os.Mkdir(path, os.ModePerm)
		return nil
	}
	if !inf.IsDir() {
		return fmt.Errorf("isn't a directory")
	}
	return nil
}
