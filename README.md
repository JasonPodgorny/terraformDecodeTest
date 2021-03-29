# decodeTest

## Overview

decodeTest is a utility to test that JSON and YAML files will decode properly for terraform.   It uses the same functions that terraform users for yamldecode and jsondecode under the hood, so if something passes this then terraform should have no issue with it.     

This allows you to test files rapidly without performing a full terraform run against them.   This can be especially helpful when you have a large number of files you are trying to decode and terraform isn't being nice about telling you which one.

You can also inject this before your terraform runs so users will get rapid feedback when formatting mistakes have happened.   

### Usage


```
Usage of C:\terraform\decodeTest\bin\decodeTest_windows_amd64_v0.1.exe.exe:
  -excludedirs value
        List of exclude dirs (default .git, .terragrunt-cache, scripts)
  -matchpatterns value
        List of match patterns (default *.json, *.yaml)
  -path string
        Path to search (default ".")
```

### Examples

All Decode Successfully

```
infra-live> decodeTest_windows_amd64_v0.1.exe

2021/03/28 22:19:30 8 total files  0.0 MB
2021/03/28 22:19:30 8 .yaml files, 0 Decode Errors
2021/03/28 22:19:30 All Files Decoded Successfully

infra-live> echo $LASTEXITCODE
0
```

Error In File

```
infra-live> decodeTest_windows_amd64_v0.1.exe

2021/03/28 22:20:41 error decoding file common_vars_global_defaults.yaml: on line 20, column 5: did not find expected key
2021/03/28 22:20:41 8 total files  0.0 MB
2021/03/28 22:20:41 8 .yaml files, 1 Decode Errors
2021/03/28 22:20:41 Decode Errors Found In Files

infra-live> echo $LASTEXITCODE
1
```

