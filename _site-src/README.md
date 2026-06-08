# BearStack Zensical Site

Diese Quellen bauen die statische BearStack-Website nach `../_site`.

```sh
python3 -m venv /tmp/bearstack-zensical-venv
/tmp/bearstack-zensical-venv/bin/python -m pip install -r _site-src/requirements.txt
/tmp/bearstack-zensical-venv/bin/zensical build -f zensical.toml --clean
```

Die Seite ist bewusst offline-freundlich konfiguriert: `use_directory_urls = false`, lokale Assets und keine externen Webfonts.
