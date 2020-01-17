package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type selectorMode int

var (
	yearReg               = regexp.MustCompile(`^\d{4}$`)
	seasonReg             = regexp.MustCompile(`s(?P<season>\d+)`)
	episodeReg            = regexp.MustCompile(`e(?P<episode>\d+)`)
	queryReg              = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	intReg                = regexp.MustCompile(`^\d+$`)
	validSingleCharTokens = []string{"a", "i", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
)

const (
	movieSelector = iota
	tvSelector
	tvSeasonEpisodeSelector
)

type Selector struct {
	mode             selectorMode
	movieDb          *MovieDb
	inDir            string
	reader           *bufio.Reader
	stopWords        []string
	tvId             int64
	seasonNumber     int
	tvSeason         TvSeason
	query            string
	tvShowSelections map[string]int64
}

func NewSelector(movieDb *MovieDb, inDir string, reader *bufio.Reader, stopWords []string) *Selector {
	return &Selector{
		mode:             movieSelector,
		movieDb:          movieDb,
		inDir:            inDir,
		reader:           reader,
		stopWords:        stopWords,
		tvId:             0,
		seasonNumber:     0,
		tvSeason:         TvSeason{},
		query:            "",
		tvShowSelections: make(map[string]int64),
	}
}

func (s *Selector) setMovieMode(query string) {
	s.mode = movieSelector
	s.tvId = 0
	s.seasonNumber = 0
	s.tvSeason = TvSeason{}
	s.query = query
}

func (s *Selector) isMovieMode() bool {
	return s.mode == movieSelector
}

func (s *Selector) setTvMode(query string) {
	s.mode = tvSelector
	s.tvId = 0
	s.seasonNumber = 0
	s.tvSeason = TvSeason{}
	s.query = query
}

func (s *Selector) isTvMode() bool {
	return s.mode == tvSelector
}

func (s *Selector) setTvSeasonEpisodeMode(tvId int64, seasonNumber int, query string) error {
	tv, err := s.movieDb.GetTv(tvId)
	if err != nil {
		return err
	}

	tvSeason, err := s.movieDb.GetTvSeason(tv, seasonNumber)
	if err != nil {
		return err
	}

	s.mode = tvSeasonEpisodeSelector
	s.tvId = tvId
	s.seasonNumber = seasonNumber
	s.tvSeason = tvSeason
	s.query = query
	s.tvShowSelections[query] = tvId
	return nil
}

func (s *Selector) isTvSeasonEpisodeMode() bool {
	return s.mode == tvSeasonEpisodeSelector
}

func (s *Selector) modeName() string {
	switch s.mode {
	case movieSelector:
		return "Movie"
	case tvSelector:
		return "Tv show"
	case tvSeasonEpisodeSelector:
		return "Episode"
	default:
		return "unknown"
	}
}

func GetQuery(moviePath, inDir string, stopWords []string) string {
	ext := filepath.Ext(moviePath)
	name := moviePath[0 : len(moviePath)-len(ext)]
	relativeName := strings.TrimPrefix(name, fmt.Sprintf("%s/", inDir))
	fileName := filepath.Base(name)
	myQuery := buildQuery(fileName, stopWords)
	testQuery, _, _, _ := extractTvSeasonEpisodeFromQuery(myQuery)

	if testQuery == "" {
		// if query is empty after extracting season/episode info,
		// use entire path inside inDir to build query
		// instead of just filename
		myQuery = buildQuery(relativeName, stopWords)
	}

	return myQuery
}

func (s *Selector) Handle(i, n int, moviePath string, common []string, info string) (Media, error) {
	myQuery := GetQuery(moviePath, s.inDir, s.stopWords)
	return s.HandleQuery(i, n, moviePath, myQuery, false, common, info, 1)
}

func (s *Selector) HandleQuery(i, n int, moviePath, query string, manual bool, common []string, info string, page int) (Media, error) {
	fmt.Println(info)

	myQuery, season, episode, year := extractTvSeasonEpisodeFromQuery(strings.TrimSpace(query))

	suffixTerms := []string{}
	if year > 0 {
		suffixTerms = append(suffixTerms, fmt.Sprintf("year: %d", year))
	}
	if season > 0 {
		suffixTerms = append(suffixTerms, fmt.Sprintf("season: %d", season))
	}
	if episode > 0 {
		suffixTerms = append(suffixTerms, fmt.Sprintf("episode: %d", episode))
	}
	displayQuerySuffix := strings.Join(suffixTerms, ", ")
	if displayQuerySuffix != "" {
		displayQuerySuffix = fmt.Sprintf(" (%s)", displayQuerySuffix)
	}

	if season == 0 && episode == 0 {
		s.setMovieMode(myQuery)
	} else if s.isMovieMode() {
		s.setTvMode(myQuery)
	} else if s.isTvSeasonEpisodeMode() && (s.seasonNumber != season || s.query != myQuery) {
		if !manual && len(common) > 0 {
			myQuery = strings.Join(common, " ")
		}
		s.setTvMode(myQuery)
	}

	if s.isTvMode() {
		if tvId, ok := s.tvShowSelections[myQuery]; ok {
			err := s.setTvSeasonEpisodeMode(tvId, season, myQuery)
			if err != nil {
				fmt.Println("Error selecting tv show based on previous query:", err)
			}
		}
	}

	var (
		results      []Media
		totalPages   int
		displayQuery string
	)

	if myQuery != "" && s.isMovieMode() {
		// search movies
		response, err := s.movieDb.SearchMovie(myQuery, page, year)
		if err != nil {
			fmt.Println("Error searching movies:", err)
		}
		results = response.MediaResults()
		totalPages = response.TotalPages
		displayQuery = fmt.Sprintf("%s%s", myQuery, displayQuerySuffix)
	} else if myQuery != "" && s.isTvMode() {
		// search tv shows
		response, err := s.movieDb.SearchTv(myQuery, page, year)
		if err != nil {
			fmt.Println("Error searching tv shows:", err)
		}
		results = response.MediaResults()
		totalPages = response.TotalPages
		displayQuery = fmt.Sprintf("%s%s", myQuery, displayQuerySuffix)
	} else if s.isTvSeasonEpisodeMode() {
		// select from episodes of known tv season
		results = s.tvSeason.MediaResults()
		totalPages = 1
		displayQuery = fmt.Sprintf("%s%s", s.tvSeason.TvName, displayQuerySuffix)
	} else {
		results = []Media{}
		totalPages = 0
		displayQuery = fmt.Sprintf("%s%s", myQuery, displayQuerySuffix)
	}

	if totalPages > 1 {
		fmt.Printf("%s query (page %d/%d): %s\n", s.modeName(), page, totalPages, ColorStr(RedColor, displayQuery))
	} else {
		fmt.Printf("%s query: %s\n", s.modeName(), ColorStr(RedColor, displayQuery))
	}

	numResults := len(results)

	if numResults == 0 {
		fmt.Println("No results!")
	}

	var defaultSelection int
	if s.isTvSeasonEpisodeMode() && episode > 0 && episode <= numResults {
		defaultSelection = episode
	} else {
		defaultSelection = 1
	}

	printMediaOptions(results)

	var selection string
	for {
		options := "qsh"
		if totalPages > 1 {
			options += "p"
		}
		if numResults <= 0 {
			fmt.Printf("[%s] ➜ ", ColorStr(RedColor, options))
		} else if numResults == 1 {
			fmt.Printf("[%s] (default: 1) ➜ ", ColorStr(RedColor, "1"+options))
		} else {
			choices := fmt.Sprintf("1-%d%s", numResults, options)
			fmt.Printf("[%s] (default: %d) ➜ ", ColorStr(RedColor, choices), defaultSelection)
		}
		rawSelection, err := s.reader.ReadString('\n')
		if err != nil {
			log.Println("Error getting selection:", err)
			continue
		}

		selection = strings.TrimSpace(rawSelection)

		if selection == "q" {
			return Movie{}, errors.New("quit")
		} else if selection == "s" {
			return Movie{}, errors.New("skipped")
		} else if selection == "p" {
			if page < totalPages {
				return s.HandleQuery(i, n, moviePath, query, manual, common, info, page+1)
			} else {
				return s.HandleQuery(i, n, moviePath, query, manual, common, info, 1)
			}
		} else if selection == "h" {
			if numResults == 1 {
				fmt.Println("1 select\ndefault (empty string) select choice 1")
			} else if numResults > 1 {
				fmt.Printf("1-%d select\ndefault (empty string) select choice %d\n", numResults, defaultSelection)
			}
			fmt.Printf(strings.TrimSpace(`
q quit
s skip
h this help
p next page of results (if available)
any other text is new query
			`) + "\n\n")
			continue
		} else {
			var iSel int
			if selection == "" {
				iSel = defaultSelection
			} else if intReg.MatchString(selection) {
				iSel, err = strconv.Atoi(selection)
				if err != nil {
					// shouldn't happen due to regex check above
					fmt.Println("Invalid selection:", err)
					continue
				}
			} else {
				// new non-selection query
				return s.HandleQuery(i, n, moviePath, selection, true, common, info, 1)
			}

			if iSel >= 1 && iSel <= numResults {
				if s.isTvMode() {
					if season > 0 {
						// we've selected a tv show, now need to select season and episode
						err = s.setTvSeasonEpisodeMode(results[iSel-1].GetId(), season, myQuery)
						if err != nil {
							fmt.Println("Invalid tv season selection:", err)
							continue
						}
						return s.HandleQuery(i, n, moviePath, query, manual, common, info, page)
					} else {
						fmt.Println("Unable to extract season number from query string.")
						continue
					}
				} else {
					// we've selected either a movie or a tv show, season & episode
					return results[iSel-1], nil
				}
			} else {
				fmt.Println("Please select one of the listed options.")
				continue
			}
		}
	}
}

func terminalWidth() (int, error) {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(out))
	if len(fields) < 2 {
		return 0, fmt.Errorf("Unable to determine terminal width")
	}
	widthStr := fields[1]
	width, err := strconv.Atoi(widthStr)
	if err != nil {
		return 0, err
	}
	return width, nil
}

func printMediaOptions(options []Media) {
	width, err := terminalWidth()
	if err != nil {
		width = 120
	}

	for i, option := range options {
		line := NewLinePrinter(width)
		line.AddColorf(YellowColor, "%2d", i+1)
		line.AddColor(WhiteColor, option.GetName())

		if option.GetDate() != "" {
			line.Addf("(%s)", option.GetDate())
		}

		overview := strings.TrimSpace(option.GetOverview())
		if overview != "" {
			line.AddFields(overview)
		}

		fmt.Println(line)
	}
}

func isQueryToken(token string, stopWords []string) bool {
	return !stringSliceContains(stopWords, token) &&
		!(len(token) == 1 && !stringSliceContains(validSingleCharTokens, token))
}

func buildQueryTokens(movieStr string, stopWords []string) []string {
	cleaned := queryReg.ReplaceAllString(movieStr, " ")
	lower := strings.ToLower(cleaned)
	words := []string{}
	for _, word := range strings.Fields(lower) {
		if isQueryToken(word, stopWords) {
			words = append(words, word)
		}
	}
	return words
}

func buildQuery(movieStr string, stopWords []string) string {
	return strings.Join(buildQueryTokens(movieStr, stopWords), " ")
}

func extractTvSeasonEpisodeFromQuery(query string) (string, int, int, int) {
	newQuery := []string{}
	season := 0
	episode := 0
	year := 0
	yearHigh := time.Now().Year() + 1

	for _, field := range strings.Fields(query) {
		var fieldSeason int
		sm := seasonReg.FindAllStringSubmatch(field, -1)
		if len(sm) > 0 && len(sm[0]) > 1 {
			fieldSeason, _ = strconv.Atoi(strings.TrimPrefix(sm[0][1], "0"))
		}

		if fieldSeason > 0 && season == 0 {
			season = fieldSeason
		}

		var fieldEpisode int
		em := episodeReg.FindAllStringSubmatch(field, -1)
		if len(em) > 0 && len(em[0]) > 1 {
			fieldEpisode, _ = strconv.Atoi(strings.TrimPrefix(em[0][1], "0"))
		}

		if fieldEpisode > 0 && episode == 0 {
			episode = fieldEpisode
		}

		var fieldYear int
		if yearReg.MatchString(field) {
			fieldYear, _ = strconv.Atoi(field)
		}

		if fieldYear >= 1900 && fieldYear <= yearHigh && year == 0 {
			year = fieldYear
		}

		if fieldSeason == 0 && fieldEpisode == 0 && fieldYear == 0 {
			newQuery = append(newQuery, field)
		}
	}

	return strings.Join(newQuery, " "), season, episode, year
}
