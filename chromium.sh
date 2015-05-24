#!/bin/bash

set -e

B=$HOME/.config/chromium/Default/Bookmarks

cat $B | grep url | grep -v \"type\": | sed -e 's/\s*\"url\": \"/,/' | sed -e 's/\"$//' | grep -v -f ~/.config/urlarchive/blacklist | urlarchive -f ~/.config/urlarchive/ua.sqlite update
