package main

import (
	"container/list"
	"encoding/csv"
	"errors"
	"io"
	"log"
	"os"
)

func readcsv(filepath string) list.List {
	rows := list.New()

	file, err := os.Open(filepath)

	if err != nil {
		log.Fatalf("Error opening file %s", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)

	for {
		rowdata, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			log.Fatalf("Error reading row %s", err)
		}
		row := list.New()

		for _, data := range rowdata {
			row.PushBack(data)
		}
		rows.PushBack(row)
	}

	return *rows

}
