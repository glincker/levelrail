#!/bin/bash
sqlite3 /tmp/test.db "PRAGMA index_list('apps');"
