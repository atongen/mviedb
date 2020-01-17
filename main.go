package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	humanize "github.com/dustin/go-humanize"
)

// build flags
var (
	Version   string = "development"
	BuildTime string = "unset"
	BuildHash string = "unset"
	GoVersion string = "unset"
	BinName   string = path.Base(os.Args[0])
)

func versionStr() string {
	return fmt.Sprintf("%s %s %s %s %s", BinName, Version, BuildTime, BuildHash, GoVersion)
}

var defaultStopWords = splitSortUniq(`
misc,dvds,dsc,x264,tv,ac3,dvdrip,720p,xvid,x0r,evo,blueray,hdrip,cm8,hive,hq,dvdscr,brrip,1080p,hdtv,h264,dl
cmrg,ipt,hc,flawl3ss,srt,bluray,web,bd,rip,x265,d3fil3r,tvnrg,hevc,d3g,ac,dd5,2hd,batv,mtg,proper
`)

// cli flags
var (
	versionFlag      = flag.Bool("v", false, "Print version information and exit")
	printTokensFlag  = flag.Bool("p", false, "Print all unique tokens used for generated search from in-directory")
	apiKeyFlag       = flag.String("api-key", "", "MovieDB api key (required)")
	inFlag           = flag.String("in", ".", "Input/source directory")
	outFlag          = flag.String("out", ".", "Output/destination directory")
	movieOutFlag     = flag.String("movie-out", "", "Output/destination directory for movies, uses 'out' if not provided")
	tvOutFlag        = flag.String("tv-out", "", "Output/destination directory for tv episodes, uses 'out' if not provided")
	manifestFlag     = flag.String("manifest", fmt.Sprintf("./%s-manifest.json", BinName), "Path to manifest file")
	setStopWordsFlag = flag.String("set-stop-words", strings.Join(defaultStopWords, ","), "CSV of words to exclude from moviedb search")
	addStopWordsFlag = flag.String("add-stop-words", "", "CSV of words to exclude from moviedb search (added to default set-stop-words list)")
	movieExtsFlag    = flag.String("movie-exts", ".mp4,.avi,.mov,.flv,.wmv,.mkv,.m4v,.mpg,.webm", "CSV of valid movie extensions")
	noColorFlag      = flag.Bool("no-color", false, "Enable if you hate fun")
	dryRunFlag       = flag.Bool("dry-run", false, "Do not copy files from in dir to out dir")
	mvFlag           = flag.Bool("mv", false, "Move files from in dir to out dir (instead of copy)")
	confirmFlag      = flag.Bool("confirm", false, "Ask for confirmation before moving or copying files")
	cleanFlag        = flag.Bool("clean", false, "List files in out dir that are candidates for removal")
)

var (
	deepCompareChunkSize = 64000
	wordReg              = regexp.MustCompile(`[^a-zA-Z0-9]+`)
)

type ManifestEntry struct {
	InFile    string    `json:"in_file"`
	OutFile   string    `json:"out_file"`
	MovieDbId int64     `json:"movie_db_id"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

func stringSliceContains(s []string, a string) bool {
	for _, b := range s {
		if a == b {
			return true
		}
	}
	return false
}

func stringSliceContainsPrefix(pp []string, s string) bool {
	for _, p := range pp {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func stringSliceHasPrefix(s []string, p string) bool {
	for _, a := range s {
		if strings.HasPrefix(a, p) {
			return true
		}
	}
	return false
}

func splitSortUniq(wordsStr string) []string {
	cleaned := wordReg.ReplaceAllString(wordsStr, " ")
	lower := strings.ToLower(cleaned)
	fields := strings.Fields(lower)
	return sortUniq(fields)
}

func sortUniq(words []string) []string {
	ret := []string{}
	sort.Strings(words)
	for i := 0; i < len(words); i++ {
		if i == 0 {
			ret = append(ret, words[i])
		} else if words[i] != words[i-1] {
			ret = append(ret, words[i])
		}
	}
	return ret
}

func lsMovies(movieDirPath string, exts []string) ([]string, error) {
	movies := []string{}

	files, err := ioutil.ReadDir(movieDirPath)
	if err != nil {
		return movies, err
	}

	for _, f := range files {
		file := filepath.Join(movieDirPath, f.Name())
		if f.IsDir() {
			dirMovies, err := lsMovies(file, exts)
			if err != nil {
				return movies, err
			}
			movies = append(movies, dirMovies...)
		} else {
			abs, err := filepath.Abs(file)
			if err != nil {
				return movies, err
			}
			if stringSliceContains(exts, filepath.Ext(abs)) {
				movies = append(movies, abs)
			}
		}
	}

	sort.Strings(movies)
	return movies, err
}

// fileExists returns whether the given file or directory exists
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func readManifest(manifestPath string) ([]ManifestEntry, error) {
	manifest := []ManifestEntry{}

	exists, err := fileExists(manifestPath)
	if err != nil {
		return manifest, err
	}

	if !exists {
		// new manifest
		return manifest, nil
	}

	f, err := os.Open(manifestPath)
	if err != nil {
		return manifest, err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return manifest, err
	}

	err = json.Unmarshal(b, &manifest)
	return manifest, err
}

func writeManifest(manifestPath string, manifest []ManifestEntry) error {
	manifestJson, err := json.MarshalIndent(manifest, "", "    ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(manifestPath, manifestJson, 0644)
}

func buildOutFile(originalPath, outDir string, media Media) (string, error) {
	ext := strings.ToLower(filepath.Ext(originalPath))
	return fmt.Sprintf("%s/%s%s", outDir, media.GetPath(), ext), nil
}

// https://stackoverflow.com/questions/21060945/simple-way-to-copy-a-file-in-golang
// CopyFile copies a file from src to dst. If src and dst files exist, and are
// the same, then return success. Otherise, attempt to create a hard link
// between the two files. If that fail, copy the file contents from src to dst.
func CopyFile(src, dst string) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return
	}
	if !sfi.Mode().IsRegular() {
		// cannot copy non-regular files (e.g., directories,
		// symlinks, devices, etc.)
		return fmt.Errorf("CopyFile: non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}
	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf("CopyFile: non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
		}
		if os.SameFile(sfi, dfi) {
			return
		}
	}
	if err = os.Link(src, dst); err == nil {
		return
	}
	err = copyFileContents(src, dst)
	return
}

func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}

// SameFile checks to see if both files share the same inode,
// if not, it falls back to DeepCompare
func SameFile(file1, file2 string) (bool, error) {
	info1, err := os.Stat(file1)
	if err != nil {
		return false, err
	}

	info2, err := os.Stat(file2)
	if err != nil {
		return false, err
	}

	if os.SameFile(info1, info2) {
		return true, nil
	}

	return DeepCompare(file1, file2)
}

// https://stackoverflow.com/questions/29505089/how-can-i-compare-two-files-in-golang
func DeepCompare(file1, file2 string) (bool, error) {
	f1, err := os.Open(file1)
	if err != nil {
		return false, err
	}

	f2, err := os.Open(file2)
	if err != nil {
		return false, err
	}

	for {
		b1 := make([]byte, deepCompareChunkSize)
		_, err1 := f1.Read(b1)

		b2 := make([]byte, deepCompareChunkSize)
		_, err2 := f2.Read(b2)

		if err1 != nil || err2 != nil {
			if err1 == io.EOF && err2 == io.EOF {
				return true, nil
			} else if err1 == io.EOF || err2 == io.EOF {
				return false, nil
			} else {
				return false, fmt.Errorf("%s, %s", err1, err2)
			}
		}

		if !bytes.Equal(b1, b2) {
			return false, nil
		}
	}
}

func movieInfo(i, n int, moviePath, inDir string) string {
	name := strings.TrimPrefix(moviePath, fmt.Sprintf("%s/", inDir))
	return fmt.Sprintf("\n%d/%d %s\n", i+1, n, ColorStr(BlueColor, name))
}

func confirm(msg string, reader *bufio.Reader) bool {
	fmt.Printf(msg)
	raw, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response := strings.TrimSpace(raw)
	if len(response) == 0 {
		return false
	}

	lower := strings.ToLower(response)
	return lower[:1] == "y"
}

// lowest directories under outDir that do not contain an out file from manifest
func getCleanDirs(outDir string, manifest []ManifestEntry) ([]string, error) {
	dirs := []string{}
	outFiles := []string{}
	for _, m := range manifest {
		if m.OutFile != "" && strings.HasPrefix(m.OutFile, outDir) {
			outFiles = append(outFiles, m.OutFile)
		}
	}

	err := filepath.Walk(outDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Printf("prevent panic by handling failure accessing a path %q: %v\n", path, err)
			return err
		}
		if info.IsDir() {
			if !stringSliceContainsPrefix(dirs, path) && !stringSliceHasPrefix(outFiles, path) {
				dirs = append(dirs, path)
			}
		}
		return nil
	})

	return dirs, err
}

func getOutDir(outFlag, fallbackOutFlag string) (string, error) {
	var out string
	if outFlag != "" {
		out = outFlag
	} else {
		out = fallbackOutFlag
	}

	outDir, err := filepath.Abs(out)
	if err != nil {
		return "", fmt.Errorf("Error getting absolute path to out dir: %s", err)
	}

	outDirExists, err := fileExists(outDir)
	if err != nil {
		return "", fmt.Errorf("Error checking out dir: %s", err)
	}

	if !outDirExists {
		return "", fmt.Errorf("Out directory does not exist!")
	}

	return outDir, nil
}

func fNameSansExtension(fPath string) string {
	ext := filepath.Ext(fPath)
	name := fPath[0 : len(fPath)-len(ext)]
	return filepath.Base(name)
}

// sortedIntersect has complexity: O(n * log(n)), a & b need to be sorted
func sortedIntersect(a []string, b []string) []string {
	set := make([]string, 0)

	for _, el := range a {
		idx := sort.SearchStrings(b, el)
		if idx < len(b) && b[idx] == el {
			set = append(set, el)
		}
	}

	return set
}

func commonDirWords(moviePath string, movieList []string, stopWords []string) ([]string, error) {
	name := fNameSansExtension(moviePath)
	peerPaths := []string{name}
	dir, err := filepath.Abs(filepath.Dir(moviePath))
	if err != nil {
		return []string{}, err
	}

	for _, p := range movieList {
		if strings.HasPrefix(p, dir) {
			peerPaths = append(peerPaths, fNameSansExtension(p))
		}
	}

	originalQueryTokens := buildQueryTokens(peerPaths[0], stopWords)
	common := sortUniq(originalQueryTokens)

	for i := 1; i < len(peerPaths); i++ {
		b := sortUniq(buildQueryTokens(peerPaths[i], stopWords))
		common = sortedIntersect(common, b)
		if len(common) == 0 {
			return common, nil
		}
	}

	// ensure original ordering
	result := []string{}
	for _, el := range originalQueryTokens {
		if stringSliceContains(common, el) {
			result = append(result, el)
		}
	}

	return result, nil
}

func main() {
	flag.Parse()

	if *versionFlag {
		fmt.Println(versionStr())
		os.Exit(0)
	}

	movieOutDir, err := getOutDir(*movieOutFlag, *outFlag)
	if err != nil {
		log.Fatalln("Movie out error:", err)
	}

	tvOutDir, err := getOutDir(*tvOutFlag, *outFlag)
	if err != nil {
		log.Fatalln("TV out error:", err)
	}

	var manifestPath string
	if *dryRunFlag && !*cleanFlag {
		manifestStr := *manifestFlag
		manifestExt := filepath.Ext(manifestStr)
		manifestSuffix := fmt.Sprintf("-dry-run%s", manifestExt)
		if strings.HasSuffix(manifestStr, manifestSuffix) {
			manifestPath = manifestStr
		} else {
			manifestName := manifestStr[0 : len(manifestStr)-len(manifestExt)]
			manifestPath = fmt.Sprintf("%s%s", manifestName, manifestSuffix)
		}
	} else {
		manifestPath = *manifestFlag
	}

	manifest, err := readManifest(manifestPath)
	if err != nil {
		log.Fatalln("Manifest error:", err)
	}

	if *cleanFlag {
		if movieOutDir != tvOutDir {
			log.Fatalln("Cannot clean differnt movie-out and tv-out at the same time")
		}
		dirs, err := getCleanDirs(movieOutDir, manifest)
		if err != nil {
			log.Fatalln("Error getting directories for cleanup")
		}
		for _, dir := range dirs {
			fmt.Println(dir)
			if !*dryRunFlag {
				os.RemoveAll(dir)
			}
		}
		os.Exit(0)
	}

	inDir, err := filepath.Abs(*inFlag)
	if err != nil {
		log.Fatalln("Error getting absolute path to in dir:", err)
	}

	exts := strings.Split(*movieExtsFlag, ",")
	movieList, err := lsMovies(inDir, exts)
	if err != nil {
		log.Fatalln("List movies error:", err)
	}

	numMovies := len(movieList)

	stopWords := strings.Split(*setStopWordsFlag, ",")
	stopWords = append(stopWords, strings.Split(*addStopWordsFlag, ",")...)
	stopWords = sortUniq(stopWords)

	if *printTokensFlag {
		tokens := []string{}
		for _, moviePath := range movieList {
			seen := false
			for _, e := range manifest {
				if e.InFile == moviePath || e.OutFile == moviePath {
					seen = true
				}
			}
			if !seen {
				query := splitSortUniq(GetQuery(moviePath, inDir, stopWords))
				myQuery, _, _, _ := extractTvSeasonEpisodeFromQuery(strings.Join(query, " "))
				tokens = append(tokens, strings.Fields(myQuery)...)
			}
		}
		for _, token := range sortUniq(tokens) {
			fmt.Println(token)
		}
		os.Exit(0)
	}

	if *apiKeyFlag == "" {
		log.Fatalln("api-key is required")
	}

	movieDb := NewMovieDb(*apiKeyFlag)

	reader := bufio.NewReader(os.Stdin)

	selector := NewSelector(movieDb, inDir, reader, stopWords)

	var verb string
	if *mvFlag {
		verb = "move"
	} else {
		verb = "copy"
	}

	for i, moviePath := range movieList {
		exists := false
		info := movieInfo(i, numMovies, moviePath, inDir)
		for _, e := range manifest {
			if e.InFile == moviePath || e.OutFile == moviePath {
				fmt.Println(info)
				fmt.Printf("Skipping because we've seen this in-file before\n\n")
				exists = true
				break
			}
		}

		if exists {
			continue
		}

		common, err := commonDirWords(moviePath, movieList, stopWords)
		if err != nil {
			log.Println("Error getting common directory query tokens:", err)
			break
		}

		movie, err := selector.Handle(i, numMovies, moviePath, common, info)
		if err != nil {
			if err.Error() == "skipped" {
				continue
			} else if err.Error() == "quit" {
				break
			} else {
				log.Println("Error searching movies:", err)
				break
			}
		}

		var outFile string
		if movie.GetType() == "tv_episode" {
			outFile, err = buildOutFile(moviePath, tvOutDir, movie)
		} else {
			outFile, err = buildOutFile(moviePath, movieOutDir, movie)
		}

		if err != nil {
			log.Println("Unable to build out file:", err)
			break
		}

		doCopy := true
		if outFile == moviePath {
			fmt.Println("In file and out file are the same path")
		} else if _, err := os.Stat(outFile); err == nil {
			// outFile exists
			isSameFile, err := SameFile(moviePath, outFile)
			if err != nil {
				log.Println("Error comparing files:", err)
				break
			}

			if isSameFile {
				fmt.Println("Out file exists and is same content as in file, updating manifest")
				doCopy = false
			} else {
				inInfo, err := os.Stat(moviePath)
				if err != nil {
					log.Println("Error getting info for in file:", err)
				}

				outInfo, err := os.Stat(outFile)
				if err != nil {
					log.Println("Error getting info for out file:", err)
				}

				fmt.Println("Out file exists and has different content as in file!")
				fmt.Println("In: ", moviePath)
				fmt.Printf("     Size: %s, modified: %s\n", humanize.Bytes(uint64(inInfo.Size())), inInfo.ModTime())
				fmt.Println("Out:", outFile)
				fmt.Printf("     Size: %s, modified: %s\n", humanize.Bytes(uint64(outInfo.Size())), outInfo.ModTime())

				if !confirm(fmt.Sprintf("%s? [yN] ➜ ", strings.Title(verb)), reader) {
					continue
				}
			}
		}

		fmt.Printf("%s %s %s %s\n", strings.Title(verb), ColorStr(RedColor, moviePath), ColorStr(WhiteColor, "➜"), ColorStr(GreenColor, outFile))

		if !*dryRunFlag && doCopy {
			if *confirmFlag {
				if !confirm(fmt.Sprintf("%s? [yN] ➜ ", strings.Title(verb)), reader) {
					continue
				}
			}
			myOutDir := filepath.Dir(outFile)
			err = os.MkdirAll(myOutDir, 0755)
			if err != nil {
				log.Println("Error creating out directory:", err)
				break
			}

			err = CopyFile(moviePath, outFile)
			if err != nil {
				log.Println("Error copying file:", err)
				break
			}

			if *mvFlag {
				err = os.Remove(moviePath)
				if err != nil {
					log.Println("Error moving file:", err)
					break
				}
			}
		}

		manifest = append(manifest, ManifestEntry{
			InFile:    moviePath,
			OutFile:   outFile,
			MovieDbId: movie.GetId(),
			Type:      movie.GetType(),
			CreatedAt: time.Now(),
		})

		err = writeManifest(manifestPath, manifest)
		if err != nil {
			log.Println("Error updating manifest: ", err)
			break
		}
	}

	fmt.Printf("\nGoodbye!\n")
}
