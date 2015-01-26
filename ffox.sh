#!/bin/bash

set -e

H=$HOME/.mozilla/firefox
dp=$(grep -B4 'Default=1' $H/profiles.ini | grep Name= | sed -e 's:^.*=::')
dpd=$H/*.$dp
#echo $dpd

# bookmarks
function get_bookmarks {
	sqlite3 $dpd/places.sqlite 'select moz_places.last_visit_date/1000000, moz_places.url from moz_places left outer join moz_bookmarks on moz_places.id = moz_bookmarks.fk where moz_bookmarks.parent is not null' | grep -v -f ~/.config/urlarchive/blacklist | sed -e 's:|:,:'
}

# history (this is not used)
function get_history {
	sqlite3 $dpd/places.sqlite "select max(moz_places.last_visit_date)/1000000, moz_places.url from moz_places left outer join moz_bookmarks on moz_places.id = moz_bookmarks.fk where moz_bookmarks.parent is null and moz_places.last_visit_date/1000000 > cast(strftime('%s', 'now', '-3 months') as integer) group by moz_places.url order by max(moz_places.last_visit_date) limit 100000" | sed -e 's:|:,:' | grep -v -f ~/.config/urlarchive/blacklist
}

get_bookmarks | urlarchive -f ~/.config/urlarchive/ua.sqlite update

