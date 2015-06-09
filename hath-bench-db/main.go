package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"cydev.ru/hath"
)

var (
	count    int64
	dbpath   string
	sizeMax  int64
	sizeMin  int64
	resMax   int
	resMin   int
	workers  int
	generate bool
	collect  bool
	bulkSize int64
)

func init() {
	flag.Int64Var(&count, "count", 100, "files to generate")
	flag.Int64Var(&bulkSize, "bulk", 10000, "bulk size")
	flag.Int64Var(&sizeMax, "size-max", 1024*100, "maximum file size in bytes")
	flag.Int64Var(&sizeMin, "size-min", 1024*5, "minimum file size in bytes")
	flag.IntVar(&resMax, "res-max", 1980, "maximum ephemeral resolution")
	flag.IntVar(&resMin, "res-min", 500, "minumum ephemeral resolution")
	flag.BoolVar(&generate, "generate", false, "generate data")
	flag.BoolVar(&collect, "collect", true, "collect old files")
	flag.StringVar(&dbpath, "dbfile", "db.bolt", "working directory")
}

func main() {
	flag.Parse()
	db, err := hath.NewDB(dbpath)
	defer db.Close()
	if err != nil {
		log.Fatal(err)
	}
	if generate {
		g := hath.FileGenerator{
			SizeMax:       sizeMax,
			SizeMin:       sizeMin,
			ResolutionMax: resMax,
			ResolutionMin: resMin,
		}

		log.Println("generating", count, "files")

		fmt.Printf("%+v\n", g)
		var i int64
		files := make([]hath.File, count)
		for i = 0; i < count; i++ {
			files[i] = g.NewFake()
		}
		start := time.Now()
		if count < bulkSize {
			log.Println("writing")
			if err := db.AddBatch(files); err != nil {
				log.Fatal(err)
			}
		} else {
			log.Println("writing in bulks")
			for i = 0; i+bulkSize < count; i += bulkSize {
				bulkstart := time.Now()

				if err := db.AddBatch(files[i : i+bulkSize]); err != nil {
					log.Fatal(err)
				}

				log.Println("from", i, "to", i+bulkSize, time.Now().Sub(bulkstart))
			}
			log.Println("from", i+bulkSize, "to", count)
			if err := db.AddBatch(files[i:]); err != nil {
				log.Fatal(err)
			}
		}
		end := time.Now()
		duration := end.Sub(start)
		rate := float64(count) / duration.Seconds()
		fmt.Printf("OK for %v at rate %f per second\n", duration, rate)
	}
	if collect {
		log.Println("collecting")
		start := time.Now()
		n, err := db.Collect(time.Now().Add(-time.Second))
		if err != nil {
			log.Fatal(err)
		}

		end := time.Now()
		duration := end.Sub(start)
		rate := float64(n) / duration.Seconds()
		fmt.Printf("Removed %d for %v at rate %f per second\n", n, duration, rate)
	}

	log.Println("OK")
}