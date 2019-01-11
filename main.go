package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

type clip struct {
	SS string
	To string
}

const helpText = `
config.json example:
-----------------------------
00:00:12 00:08:00
00:09:00 00:23:10
...
-----------------------------
`

const (
	ffmpeg = "ffmpeg"
)

func ParseTime(str string) (int, error) {

	times := strings.Split(str, ":")
	if len(times) > 3 {
		return 0, fmt.Errorf("invalid format of time:%s", str)
	}
	sec := 0
	for i := len(times) - 1; i >= 0; i-- {
		t, err := strconv.Atoi(times[i])
		if err != nil {
			return 0, err
		}
		sec += t * int(math.Pow(60, float64(len(times)-i-1)))
	}
	return sec, nil
}

func readConfig(r io.Reader) ([]*clip, error) {
	clips := make([]*clip, 0, 10)
	br := bufio.NewReader(r)
	for {
		//l, err := br.ReadString('\n')
		l, _, err := br.ReadLine()
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			break
		}
		tt := strings.Split(string(l), " ")
		if len(tt) != 2 {
			return nil, fmt.Errorf("invalid format of time:%s", l)
		}
		c := clip{SS: tt[0], To: tt[1]}
		clips = append(clips, &c)
	}
	return clips, nil
}

var conf = flag.String("f", "config.json", `cut config`)
var i = flag.String("i", "", `input file`)
var o = flag.String("o", "", `output file`)

func main() {
	flag.Usage = func() {
		flag.PrintDefaults()
		fmt.Println(helpText)
	}
	flag.Parse()
	if *i == "" {
		log.Fatalln("no input file")
	}
	if *o == "" {
		log.Fatalln("no outfile file")
	}

	f, err := os.Open(*conf)
	if err != nil {
		log.Fatalln(err)
		return
	}
	defer f.Close()
	clips, err := readConfig(f)
	if err != nil {
		log.Fatalln(err)
	}

	tmpDir, err := ensureTmpDir(*i)
	if err != nil {
		log.Fatalln(err)
	}

	//ffmpeg -i out.ogv -s 640x480 -b 500k -vcodec h264 -r 29.97 -acodec libfaac -ab 48k -ac 2 out.mp4
	convertTarget := filepath.Join(tmpDir, "720p"+*i)
	args := []string{"-i", *i, "-s", "1280x720", "-b", "1500k", convertTarget}
	log.Println("convert", *i, "to", convertTarget)
	err = convert(args...)
	if err != nil {
		log.Fatalln("convert failed", err)
	}
	log.Println("convert success")
	log.Println("split video")

	clipspath, err := split(clips, tmpDir, convertTarget)
	if err != nil {
		log.Fatalln("split failed", err)
	}
	err = concat(clipspath, tmpDir, *o)
	if err != nil {
		log.Fatalln(err)
	}
}

func ensureTmpDir(input string) (string, error) {
	tmpDir := strings.Split(input, ".")[0] + "_tmp"
	err := os.MkdirAll(tmpDir, os.ModePerm)
	return tmpDir, err
}

func convert(args ...string) error {
	cmd := exec.Command(ffmpeg, args...)
	output := args[len(args)-1]
	_, err := os.Stat(output)
	if err == nil {
		return nil
	}
	return cmd.Run()
}

func split(clips []*clip, workDir, input string) ([]string, error) {
	wg := sync.WaitGroup{}
	outputs := make([]string, 0, len(clips))
	//input = filepath.Join(workDir, input)
	for i, c := range clips {
		wg.Add(1)
		clipOut := filepath.Join(workDir, fmt.Sprintf("%d.mp4", i))
		go func(c *clip, clipOut string) {
			defer wg.Done()

			//ffmpeg -i input.wmv -ss 00:00:30.0 -c copy -t 00:00:10.0 output.wmv
			args := []string{"-i", input, "-ss", c.SS, "-c", "copy", "-to", c.To, clipOut}
			fmt.Println(args)
			cmd := exec.Command(ffmpeg, args...)
			cmd.Run()
			outputs = append(outputs, clipOut)
		}(c, clipOut)
	}
	wg.Wait()
	return outputs, nil
}

// $ ffmpeg -f concat -i mylist.txt -c copy output
// $ cat mylist.txt
// file '/path/to/file1'
// file '/path/to/file2'
// file '/path/to/file3'
func concat(clips []string, workdir, output string) error {

	pwd, _ := os.Getwd()

	buf := bytes.Buffer{}
	for _, v := range clips {
		buf.WriteString("file '" + filepath.Join(pwd, v) + "'\n")
	}
	clipsfile := filepath.Join(workdir, "clips-"+output+".txt")
	if err := ioutil.WriteFile(clipsfile, buf.Bytes(), 0664); err != nil {
		return err
	}

	args := []string{"-f", "concat", "-safe", "0", "-i", clipsfile, "-c", "copy", output}
	log.Println(args)

	cmd := exec.Command(ffmpeg, args...)
	return cmd.Run()
}
