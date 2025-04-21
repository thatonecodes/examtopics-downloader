# Exam Topics Downloader

This repo aims to make it possible for you to obtain all the exam questions from the examtopics website (which is paywalled).

## Setting it Up

### Using docker

1. Make sure [docker](https://docs.docker.com/engine/install/) is installed on your system.
2. Pull the docker image:

```bash
docker pull ghcr.io/thatonecodes/examtopics-downloader:latest
```

3\. Run the container:

```bash
docker run -d \
 --name examtopics-downloader \
 -p google -s devops \
 -save-links -o output.md \
 ghcr.io/thatonecodes/examtopics-downloader:latest
```

> [!NOTE]  
> The built binary from the `Dockerfile` has the name `examtopicsdl`.

### Building from Source

1. First, you must install [Golang >= 1.24](https://go.dev/doc/install) from the offical website.
2. Then, run `git clone https://github.com/thatonecodes/examtopics-downloader` in your terminal to clone the repo.
3. `cd` into the directory: `cd examtopics-downloader`
4. You can now run: `go run . (args...)`

(there will be compiled binaries in the future)

## Command Line Arguments

```
Each command line argument you can provide when running the program:

  -c	Optionally include all the comment/discussion text
  -exams
    	Optionally show all the possible exams for your selected provider and exit
  -o string
    	Optional path of the file where the data will be outputted (default "examtopics_output.md")
  -p string
    	Name of the exam provider (default "google")
  -s string
    	String to grep for in discussion links (required)
  -save-links
    	Optional argument to save unique links to questions
```

## Possible Arguments List

### Exam Providers, `-p`

| Provider (-p)    | View Exams                                                                     | Notes     |
| ---------------- | ------------------------------------------------------------------------------ | --------- |
| amazon           | [Amazon Exams](https://www.examtopics.com/exams/amazon/)                       | AWS Certs |
| cisco            | [Cisco Exams](https://www.examtopics.com/exams/cisco/)                         |           |
| comptia          | [CompTIA Exams](https://www.examtopics.com/exams/comptia/)                     |           |
| salesforce       | [Salesforce Exams](https://www.examtopics.com/exams/salesforce/)               |           |
| fortinet         | [Fortinet Exams](https://www.examtopics.com/exams/fortinet/)                   |           |
| juniper          | [Juniper Exams](https://www.examtopics.com/exams/juniper/)                     |           |
| isaca            | [ISACA Exams](https://www.examtopics.com/exams/isaca/)                         |           |
| vmware           | [VMware Exams](https://www.examtopics.com/exams/vmware/)                       |           |
| isc2             | [ISC2 Exams](https://www.examtopics.com/exams/isc2/)                           | CISSP etc |
| servicenow       | [ServiceNow Exams](https://www.examtopics.com/exams/servicenow/)               |           |
| google           | [Google Exams](https://www.examtopics.com/exams/google/)                       |           |
| microsoft        | [Microsoft Exams](https://www.examtopics.com/exams/microsoft/)                 |           |
| ec-council       | [EC-Council Exams](https://www.examtopics.com/exams/ec-council/)               | CEH etc   |
| oracle           | [Oracle Exams](https://www.examtopics.com/exams/oracle/)                       |           |
| paloaltonetworks | [Palo Alto Networks Exams](https://www.examtopics.com/exams/paloaltonetworks/) |           |

> [!NOTE]  
> The more the amount of exams/discussion the provider has, the longer it will take to scrape through the exams.

### `-save-links` && `-output-save-links`

This is a bool flag, so the default is that it's set to `false`, deactivated. If `-save-links` is false `-output-save-links` will do nothing.
`-output-save-links` is a `string` which includes the output path for the saved links, default is `saved-links.txt`.

### Grep String, `-s`

The `-s` argument can take an exam ID (ex. 200-301) or a word, such as "devops". for example:

```bash
go run . -p google -s devops
```

would get all exams from the `google` provider containing the string `devops`.

### Comments and output, `-c` && `-o`

The `-c` argument is another bool flag, so it is defaultly set to false(as it creates a lot of noise in the `.md` file), but you can include it by adding the flag.
`-o` is the output path, based on `os.create(path)`, in the current working directory.

### Exams output, `-exams`

This argument will display output defaulted to such as and exit immediately.

```
Exams for provider 'google'

https://www.examtopics.com/exams/google/adwords-fundamentals/
https://www.examtopics.com/exams/google/associate-android-developer/
https://www.examtopics.com/exams/google/associate-cloud-engineer/
https://www.examtopics.com/exams/google/associate-data-practitioner/
https://www.examtopics.com/exams/google/associate-google-workspace-administrator/
https://www.examtopics.com/exams/google/cloud-digital-leader/
https://www.examtopics.com/exams/google/display-advertising/
https://www.examtopics.com/exams/google/google-analytics/
https://www.examtopics.com/exams/google/gsuite/
https://www.examtopics.com/exams/google/individual-qualification/
https://www.examtopics.com/exams/google/mobile-advertising/
https://www.examtopics.com/exams/google/professional-chromeos-administrator/
https://www.examtopics.com/exams/google/professional-cloud-architect/
https://www.examtopics.com/exams/google/professional-cloud-database-engineer/
https://www.examtopics.com/exams/google/professional-cloud-developer/
https://www.examtopics.com/exams/google/professional-cloud-devops-engineer/
https://www.examtopics.com/exams/google/professional-cloud-network-engineer/
https://www.examtopics.com/exams/google/professional-cloud-security-engineer/
https://www.examtopics.com/exams/google/professional-collaboration-engineer/
https://www.examtopics.com/exams/google/professional-data-engineer/
https://www.examtopics.com/exams/google/professional-google-workspace-administrator/
https://www.examtopics.com/exams/google/professional-machine-learning-engineer/
https://www.examtopics.com/exams/google/search-advertising/
https://www.examtopics.com/exams/google/shopping-advertising/
https://www.examtopics.com/exams/google/video-advertising/
```

## Example

So, you have installed `go` on your system, and you're inside of the working directory. Let's say you would like the questions for the cisco exam 200-301.

Open your terminal and run:

```bash
go run . -p cisco -s 200-301
```

Note that you can put the id as the string to look for, as the program is compatible this way also.

After waiting a few moments, you would see the output end with:

```bash
Successfully saved output to {OUTPUT_LOCATION}.
```

If so, hooray, you have successfully saved all/most of the questions in a `.md` file!
The format would be such as:

```
----------------------------------------

## Exam 200-301 topic 1 question 532 discussion

Actual exam question from

Cisco's
200-301

Question #: 532
Topic #: 1

[All 200-301 Questions]

Refer to the exhibit. An engineer configured NAT translations and has verified that the configuration is correct. Which IP address is the source IP after the NAT has taken place?
Suggested Answer: D 🗳️

A. 10.4.4.4

B. 10.4.4.5

C. 172.23.103.10

D. 172.23.104.4

**Answer: D**

**Timestamp: Jan. 5, 2021, 9:48 p.m.**

[View on ExamTopics](https://www.examtopics.com/discussions/cisco/view/41599-exam-200-301-topic-1-question-532-discussion/)

----------------------------------------
```

More options for formatting are coming soon.
