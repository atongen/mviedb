package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	userAgent = "mviedb https://github.com/atongen/mviedb"
	urlBase   = "https://api.themoviedb.org"
)

type cacheResult struct {
	body      []byte
	createdAt time.Time
}

type MovieDb struct {
	ApiKey                string
	Client                http.Client
	cache                 map[string]cacheResult
	cacheRetensionSeconds float64
}

type Media interface {
	GetId() int64
	GetName() string
	GetDate() string
	GetOverview() string
	GetPath() string
	GetType() string
}

type Movie struct {
	Id               int64   `json:"id"`
	Title            string  `json:"title"`
	ReleaseDate      string  `json:"release_date"`
	Popularity       float64 `json:"popularity"`
	Video            bool    `json:"video"`
	VoteCount        int     `json:"vote_count"`
	VoteAverage      float64 `json:"vote_average"`
	OriginalLanguage string  `json:"original_language"`
	OriginalTitle    string  `json:"original_title"`
	GenreIds         []int64 `json:"genre_ids"`
	BackdropPath     string  `json:"backdrop_path"`
	Adult            bool    `json:"adult"`
	Overview         string  `json:"overview"`
	PosterPath       string  `json:"poster_path"`
}

func (m Movie) GetId() int64 {
	return m.Id
}

func (m Movie) GetName() string {
	return m.Title
}

func (m Movie) GetDate() string {
	return m.ReleaseDate
}

func (m Movie) GetOverview() string {
	return m.Overview
}

func (m Movie) GetPath() string {
	dateParts := strings.Split(m.ReleaseDate, "-")
	year := dateParts[0]
	return fmt.Sprintf("%s (%s)/%s (%s)", m.Title, year, m.Title, year)
}

func (m Movie) GetType() string {
	return "movie"
}

type Tv struct {
	Id               int64    `json:"id"`
	Name             string   `json:"name"`
	OriginalName     string   `json:"original_name"`
	PosterPath       string   `json:"poster_path"`
	Popularity       float64  `json:"popularity"`
	BackdropPath     string   `json:"backdrop_path"`
	VoteAverage      float64  `json:"vote_average"`
	VoteCount        int      `json:"vote_count"`
	Overview         string   `json:"overview"`
	FirstAirDate     string   `json:"first_air_date"`
	OriginCountry    []string `json:"origin_country"`
	GenreIds         []int    `json:"genre_ids"`
	OriginalLanguage string   `json:"original_language"`
}

func (m Tv) GetId() int64 {
	return m.Id
}

func (m Tv) GetName() string {
	return m.Name
}

func (m Tv) GetDate() string {
	return m.FirstAirDate
}

func (m Tv) GetOverview() string {
	return m.Overview
}

func (m Tv) GetPath() string {
	panic("No path for tv")
}

func (m Tv) GetType() string {
	return "tv"
}

type TvSeason struct {
	Id           int64       `json:"id"`
	Name         string      `json:"name"`
	AirDate      string      `json:"air_date"`
	Episodes     []TvEpisode `json:"episodes"`
	Overview     string      `json:"overview"`
	PosterPath   string      `json:"poster_path"`
	SeasonNumber int         `json:"season_number"`
	TvName       string
}

func (r TvSeason) MediaResults() []Media {
	results := make([]Media, len(r.Episodes))
	for i, v := range r.Episodes {
		results[i] = Media(v)
	}
	return results
}

type TvEpisode struct {
	Id             int64   `json:"id"`
	Name           string  `json:"name"`
	AirDate        string  `json:"air_date"`
	EpisonNumber   int     `json:"episode_number"`
	SeasonNumber   int     `json:"season_number"`
	Overview       string  `json:"overview"`
	ProductionCode string  `json:"production_code"`
	StillPath      string  `json:"still_path"`
	VoteAverage    float64 `json:"vote_average"`
	VoteCount      int     `json:"vote_count"`
	TvName         string
	SeasonName     string
	FirstAirDate   string
}

func (m TvEpisode) GetId() int64 {
	return m.Id
}

func (m TvEpisode) GetName() string {
	return m.Name
}

func (m TvEpisode) GetDate() string {
	return m.AirDate
}

func (m TvEpisode) GetOverview() string {
	return m.Overview
}

func (m TvEpisode) GetPath() string {
	dateParts := strings.Split(m.FirstAirDate, "-")
	year := dateParts[0]
	return fmt.Sprintf("%s (%s)/%s (%s) S%02dE%02d", m.TvName, year, m.TvName, year, m.SeasonNumber, m.EpisonNumber)
}

func (m TvEpisode) GetType() string {
	return "tv_episode"
}

type SearchMovieResponse struct {
	Page         int     `json:"page"`
	Results      []Movie `json:"results"`
	TotalResults int     `json:"total_results"`
	TotalPages   int     `json:"total_pages"`
}

func (r SearchMovieResponse) MediaResults() []Media {
	results := make([]Media, len(r.Results))
	for i, v := range r.Results {
		results[i] = Media(v)
	}
	return results
}

type SearchTvResponse struct {
	Page         int  `json:"page"`
	Results      []Tv `json:"results"`
	TotalResults int  `json:"total_results"`
	TotalPages   int  `json:"total_pages"`
}

func (r SearchTvResponse) MediaResults() []Media {
	results := make([]Media, len(r.Results))
	for i, v := range r.Results {
		results[i] = Media(v)
	}
	return results
}

func NewMovieDb(apiKey string) *MovieDb {
	return &MovieDb{
		ApiKey: apiKey,
		Client: http.Client{
			Timeout: time.Second * 5,
		},
		cache: make(map[string]cacheResult),
		cacheRetensionSeconds: 60.0,
	}
}

func (c *MovieDb) cacheGet(key, url string) ([]byte, error) {
	keys := make([]string, len(c.cache))
	for k := range c.cache {
		keys = append(keys, k)
	}
	for _, k := range keys {
		entry := c.cache[k]
		age := time.Since(entry.createdAt)
		if age.Seconds() > c.cacheRetensionSeconds {
			delete(c.cache, k)
		}
	}

	if cacheResult, ok := c.cache[key]; ok {
		return cacheResult.body, nil
	}

	response := []byte{}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return response, err
	}

	req.Header.Set("User-Agent", userAgent)

	res, err := c.Client.Do(req)
	if err != nil {
		return response, err
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return response, fmt.Errorf("API request error (%s)\n", res.Status)
	}

	responseBody, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return response, err
	}

	c.cache[key] = cacheResult{responseBody, time.Now()}
	return responseBody, nil
}

func (c *MovieDb) SearchMovie(query string, page, year int) (SearchMovieResponse, error) {
	response := SearchMovieResponse{}

	url, err := searchMovieUrl(c.ApiKey, query, page, year)
	if err != nil {
		return response, err
	}

	body, err := c.cacheGet(fmt.Sprintf("search-movie-%s-%d", query, page), url)
	if err != nil {
		return response, err
	}

	err = json.Unmarshal(body, &response)
	return response, err
}

func (c *MovieDb) SearchTv(query string, page, year int) (SearchTvResponse, error) {
	response := SearchTvResponse{}

	url, err := searchTvUrl(c.ApiKey, query, page, year)
	if err != nil {
		return response, err
	}

	body, err := c.cacheGet(fmt.Sprintf("search-tv-%s-%d", query, page), url)
	if err != nil {
		return response, err
	}

	err = json.Unmarshal(body, &response)
	return response, err
}

func (c *MovieDb) GetMovie(movieId int64) (Movie, error) {
	movie := Movie{}

	url, err := movieUrl(c.ApiKey, movieId)
	if err != nil {
		return movie, err
	}

	body, err := c.cacheGet(fmt.Sprintf("get-movie-%d", movieId), url)
	if err != nil {
		return movie, err
	}

	err = json.Unmarshal(body, &movie)
	return movie, err
}

func (c *MovieDb) GetTv(tvId int64) (Tv, error) {
	tv := Tv{}

	url, err := tvUrl(c.ApiKey, tvId)
	if err != nil {
		return tv, err
	}

	body, err := c.cacheGet(fmt.Sprintf("get-tv-%d", tvId), url)
	if err != nil {
		return tv, err
	}

	err = json.Unmarshal(body, &tv)
	return tv, err
}

func (c *MovieDb) GetTvSeason(tv Tv, seasonNumber int) (TvSeason, error) {
	tvSeason := TvSeason{}

	url, err := tvSeasonUrl(c.ApiKey, tv.Id, seasonNumber)
	if err != nil {
		return tvSeason, err
	}

	body, err := c.cacheGet(fmt.Sprintf("get-tv-season-%d-%d", tv.Id, seasonNumber), url)
	if err != nil {
		return tvSeason, err
	}

	err = json.Unmarshal(body, &tvSeason)
	if err != nil {
		return tvSeason, err
	}

	tvSeason.TvName = tv.Name

	episodes := make([]TvEpisode, len(tvSeason.Episodes))
	for i := 0; i < len(tvSeason.Episodes); i++ {
		episode := tvSeason.Episodes[i]
		episode.TvName = tv.Name
		episode.SeasonName = tvSeason.Name
		episode.FirstAirDate = tv.FirstAirDate
		episodes[i] = episode
	}
	tvSeason.Episodes = episodes

	return tvSeason, err
}

func movieUrl(apiKey string, movieId int64) (string, error) {
	u, err := url.Parse(fmt.Sprintf("%s/3/movie/%d", urlBase, movieId))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("api_key", apiKey)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func tvUrl(apiKey string, tvId int64) (string, error) {
	u, err := url.Parse(fmt.Sprintf("%s/3/tv/%d", urlBase, tvId))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("api_key", apiKey)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func tvSeasonUrl(apiKey string, tvId int64, seasonNumber int) (string, error) {
	u, err := url.Parse(fmt.Sprintf("%s/3/tv/%d/season/%d", urlBase, tvId, seasonNumber))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("api_key", apiKey)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func searchMovieUrl(apiKey string, query string, page, year int) (string, error) {
	u, err := url.Parse(fmt.Sprintf("%s/3/search/movie", urlBase))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("api_key", apiKey)
	q.Set("query", query)
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func searchTvUrl(apiKey string, query string, page, year int) (string, error) {
	u, err := url.Parse(fmt.Sprintf("%s/3/search/tv", urlBase))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("api_key", apiKey)
	q.Set("query", query)
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if year > 0 {
		q.Set("year", strconv.Itoa(year))
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
