#!/usr/bin/env ruby

require "fileutils"
require "digest/md5"

unless File.exist?("testfiles.txt")
  STDERR.puts("Please populate testfiles.txt")
  exit(1)
end

files = File.open("testfiles.txt").read.split("\n")

FileUtils.rm_rf("testdata") if File.exists?("testdata")
%w[testdata testdata/movies testdata/tv].each { |d| FileUtils.mkdir(d) }
Dir.chdir("testdata")

files.each do |f|
  parts = f.split(File::SEPARATOR)
  l = parts.length
  l.times do |i|
    if i >= l - 1 # last
      unless File.exists?(f)
        File.open(f, "w") { |io| io.print(Digest::MD5.hexdigest(f)) }
      end
    else
      d = parts[0, i + 1].join(File::SEPARATOR)
      FileUtils.mkdir(d) unless File.exists?(d)
    end
  end
end
