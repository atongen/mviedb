# mviedb

`mviedb` is a command-line application that leverages the themoviedb.org api to help make organizing
your kodi media files as painless as possible. Handles both movies and tv shows.

## binary installation

Download the latest binary for your platform from the [releases](https://github.com/atongen/mviedb/releases) folder.

## compilation

Requires a golang build environment.

```
git clone https://github.com/atongen/mviedb.git
cd mviedb
make
```

## cli options

```
$ mviedb -h
Usage of dist/mviedb-0.1.0-linux-amd64:
  -add-stop-words string
    	CSV of words to exclude from moviedb search (added to default set-stop-words list)
  -api-key string
    	MovieDB api key (required)
  -clean
    	List files in out dir that are candidates for removal
  -confirm
    	Ask for confirmation before moving or copying files
  -dry-run
    	Do not copy files from in dir to out dir
  -in string
    	Input/source directory (default ".")
  -manifest string
    	Path to manifest file (default "./mviedb-0.1.0-linux-amd64-manifest.json")
  -movie-exts string
    	CSV of valid movie extensions (default ".mp4,.avi,.mov,.flv,.wmv,.mkv,.m4v,.mpg,.webm")
  -movie-out string
    	Output/destination directory for movies, uses 'out' if not provided
  -mv
    	Move files from in dir to out dir (instead of copy)
  -no-color
    	Enable if you hate fun
  -out string
    	Output/destination directory (default ".")
  -set-stop-words string
    	CSV of words to exclude from moviedb search (default "1080p,2hd,720p,ac,ac3,batv,bd,blueray,bluray,brrip,cm8,cmrg,d3fil3r,d3g,dd5,dl,dsc,dvdrip,dvds,dvdscr,evo,flawl3ss,h264,hc,hdrip,hdtv,hevc,hive,hq,ipt,misc,mtg,proper,rip,srt,tv,tvnrg,web,x0r,x264,x265,xvid")
  -tv-out string
    	Output/destination directory for tv episodes, uses 'out' if not provided
  -v	Print version information and exit
```

## usage

The process is more efficient if you assemble a good list of stop-words for your input files before you begin moving them:

```
$ mviedb \
  -in /media/movies/new \
  -add-stop-words additional,stop,words \
  -p
```

This will display a list of tokens from the input files that will be used for automatically generating moviedb.org search queries.

Added stop words will be taken into account. Repeat and continue updating `-add-stop-words` (or `-set-stop-words`) until you are happy with the result.

Next, you can begin renaming your media files. It is safe to use the same directory for input and output.

```
$ mviedb \
  -in /media/movies/new \
  -movie-out media/movies/dvd/movies \
  -tv-out /media/movies/tv \
  -manifest $HOME/mviedb-manifest.json \
  -add-stop-words killers,dimension,internal
  -mv
```

You will be prompted to search themoviedb.org based on queries generated from the input media files.

Numbers that look like years (ie. 1900 through the current year) will be extracted from the search query and used as an api `year` parameter.

Query strings that match the pattern `sXXeXX` will be extracted and used to pre-populate the season and episode information for tv show searches.

## contributing

Pull requests welcome!

Issues can be reported here: https://github.com/atongen/mviedb/issues
