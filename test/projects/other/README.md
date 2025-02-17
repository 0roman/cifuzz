## Project Description

This is an extended example project categorized under build system "other".
It includes two fuzz tests
`main/tests/explore_me_fuzztest` and `fuzztests/test_me_fuzztest` in a multiple package
setup and one corresponding Makefile.

## Run

### Heap Buffer Overflow and Undefined Behaviour

```
cifuzz run main/tests/explore_me_fuzztest
```

### Heap Use After Free

```
cifuzz run fuzztests/test_me_fuzztest
```

## Bundle

### explore_me_fuzztest

```
cifuzz bundle main/tests/explore_me_fuzztest
```

Should include the fuzz test with two targets to cover fuzzing and coverage builds.

```
...
- target: main/tests/explore_me_fuzztest
  path: libfuzzer/address+undefined/main/tests/explore_me_fuzztest/bin/main/tests/explore_me_fuzztest
...
- target: main/tests/explore_me_fuzztest
  path: replayer/coverage/main/tests/explore_me_fuzztest/bin/main/tests/explore_me_fuzztest
...
```

### test_me_fuzztest

```
cifuzz bundle fuzztests/test_me_fuzztest
```

```
...
- target: fuzztests/test_me_fuzztest
  path: libfuzzer/address+undefined/fuzztests/test_me_fuzztest/bin/fuzztests/test_me_fuzztest
...
- target: fuzztests/test_me_fuzztest
  path: replayer/coverage/fuzztests/test_me_fuzztest/bin/fuzztests/test_me_fuzztest
...
```

## Coverage

### explore_me_fuzztest

```
cifuzz coverage main/tests/explore_me_fuzztest
```

```
                              File | Functions Hit/Found |  Lines Hit/Found | Branches Hit/Found
           main/src/explore_me.cpp |      1 / 1 (100.0%) | 15 / 15 (100.0%) |     8 / 8 (100.0%)
main/tests/explore_me_fuzztest.cpp |      2 / 2 (100.0%) |   8 / 8 (100.0%) |     0 / 0 (100.0%)
                                   |                     |                  |
                                   | Functions Hit/Found |  Lines Hit/Found | Branches Hit/Found
                             Total |               3 / 3 |          23 / 23 |              8 / 8

```

### test_me_fuzztest

```
cifuzz coverage fuzztests/test_me_fuzztest
```

```
                          File | Functions Hit/Found | Lines Hit/Found | Branches Hit/Found
fuzztests/test_me_fuzztest.cpp |      2 / 2 (100.0%) |  6 / 6 (100.0%) |     0 / 0 (100.0%)
               lib/test_me.cpp |      1 / 1 (100.0%) |  9 / 9 (100.0%) |     2 / 2 (100.0%)
                               |                     |                 |
                               | Functions Hit/Found | Lines Hit/Found | Branches Hit/Found
                         Total |               3 / 3 |         15 / 15 |              2 / 2
```
