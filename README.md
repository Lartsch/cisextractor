# cisextractor
Tool to extract rules from any CIS benchmark PDF, written in Go.  
The tool had a success rate of 100% for all of the tested PDFs as of May 2022 (considering the amount of rules found and successful content extraction). Some rules might still be missed (please report any issues).  
Supports CSV and YAML output, preserves linebreaks by default.  

**PLEASE NOTE:** Some AVs detect the binaries as a trojan for an unknown reason. Please check the source and build script for yourself and create an issue if you have an idea why.  

## Installation
1.  Download the prebuilt executable for your OS (see [Releases](https://github.com/lartsch/cisextractor/releases))
### Linux (Tested)
2.  Add execution permissions to the file (```chmod +x <file>```)
3.  Install poppler-utils:
	-  ```sudo apt install poppler-utils``` or
	-  ```sudo pacman -S poppler```
4.  Run the tool in your favourite console
### Windows (Tested)
2.  Unblock the file in Powershell (```Unblock-File <file>```)
3.  Install poppler-utils:
	- Download + extract the latest package from [here](http://blog.alivate.com.au/poppler-windows/)
	- Add the *bin/* directory to your $PATH
4.  Run the tool in your favourite console
    -  If you have any anti virus software, it might take some time for it to run (whitelist it / confirm the notification)
### Mac (Untested)
2.  Add execution permissions to the file (```chmod +x <file>```)
3.  Install poppler-utils:
	-  ```brew install poppler```
4.  Run the tool in your favourite console

## Usage
```
usage: cisextractor_windows_amd64.exe --in=IN [<flags>]                                                                                                                 Flags:                   
        --help          Show context-sensitive help (also try --help-long and --help-man).
    -i, --in=IN         Filepath to parse (PDF)
    -o, --out=OUT       Optional: Output to filepath (default: ./<pdfname>.<csv|yaml>)
    -t, --trimsections  Optional: Remove all new line characters from section content (default=disabled)
    -c, --csv           Optional: Export in CSV format (default=YAML)
    -d, --details       Optional: Shows details for identification errors if needed (default=disabled)
```

**Additional notes:**  
- It can happen that some rules are not identified or there are issues with extracting their respective contents - especially when there are fundamental changes to the PDF files that break the tool's complex regular expressions. Currently, no issues were found for the latest CIS PDFs.
- Formatting of section contents does not always look good, even though lots of processing is done
	- Use the ```-t``` flag if you want to remove all linebreaks (this will lead to confusion when actually reading the content for some sections)
- Formatting of the "CIS Controls" table is not good yet - maybe additional processing will be added in the future
- ONLY the pdftotext executable found in poppler-utils will work!

## Notes for Excel import
1.  Add the ```-c``` flag when running the tool to use CSV mode
2.  In Excel, go to the *Data* tab and select *From text/CSV* then choose the generated CSV file
3.  In the import dialogue, choose *UTF-8 Unicode* as charset, make sure *Comma* is selected as delimiter and then click *Transform data*
4.  In the PowerQuery editor, select the first column (ID), go to the *Transform* tab and then choose *Text* as the *Data type*
5.  Return to the *Start* tab and hit *Close & load* to load the data to the current worksheet

![preview](https://github.com/lartsch/cisextractor/blob/main/preview.png?raw=true)

## Notes for compilation
- This tool was developed with Go version 1.17
- Simply clone this repository, run ```go mod tidy``` in the source folder and you should be good to go.
    - In case of issue, delete go.mod and run ```go mod init <modulename>``` first
- Use ```go run cisextractor.go``` to run directly or ```go build``` to build the executable for your OS
	- You can build for other architectures using the usual Go flow.
