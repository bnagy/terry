# terry

## About

This is a very early commit. It works, but there's not much doc yet. Terry gets tests from radamsa and does instrumentation ( crash analysis and triage ) with (francis)[https://github.com/bnagy/francis].

```
  Usage: terry -src /path/to/corpus -dest /path/to/workdir -- /path/to/target -infile @@ -foo -quux
  OR: terry -server 192.168.1.1:4141 -dest /path/to/workdir -- /path/to/target -infile @@ -foo -quux
  ( @@ will be substituted for each testfile )

  -dest="": directory to use to stage tests to disk
  -fix="": Unix socket to use to fix tests
  -fn=".cur_input": filename to use
  -server="": remote radamsa server to use for tests
  -src="": directory with test corpus
  -t=-1: timeout in secs for app under test
  ```

## Contributing

Fork and send a pull request.

Report issues.

## License & Acknowledgements

BSD style, see LICENSE file for details.