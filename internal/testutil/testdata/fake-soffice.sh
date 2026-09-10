#!/bin/sh
format=""
outdir=""
source=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		--convert-to)
			shift
			format="$1"
			;;
		--outdir)
			shift
			outdir="$1"
			;;
		*)
			source="$1"
			;;
	esac
	shift
done
base=$(basename "$source")
stem=${base%.*}
case "$format" in
	txt*) cp "$source" "$outdir/$stem.txt" ;;
	pdf*) printf '%s' '%PDF fake' > "$outdir/$stem.pdf" ;;
	*) exit 2 ;;
esac
