# Urlarchiver
Urlarchiver is a program to permanently store the content of your firefox (and, in theory chrome) bookmarks locally.

To use Urlarchiver just copy the program urlarchive and ffox.sh somewhere into your path run these commands:

	mkdir -p ~/.config/urlarchive/
	touch ~/.config/urlarchive/blacklist
	ffox.sh
	
Urlarchive will extract your bookmarks and archive them inside `~/.config/urlarchive/ua.sqlte`. 

Too see the archived content or do a fulltext search in them run:

	urlarchive serve
