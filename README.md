# Exam Topics Downloader

This repo aims to make it possible for you to obtain all the exam questions from the examtopics website (which is paywalled).

## Setting it Up

1. First, you must install [Golang >= 1.24](https://go.dev/doc/install) from the offical website.
2. Then, run `git clone https://github.com/thatonecodes/examtopics-downloader` in your terminal to clone the repo.
3. `cd` into the directory: `cd examtopics-downloader`
4. You can now run: `go run . (args...)`

(there will be compiled binaries in the future)

## Command Line Arguments

```bash
Each command line argument you can provide when running the program:

  -c	Optionally include all the comment/discussion text
  -o string
    	Optional path of the file where the data will be outputted (default "examtopics_output.md")
  -p string
    	Name of the exam provider (default -> google) (default "google")
  -s string
    	String to grep for in discussion links (required)
  -save-links
    	Optional argument to save unique links to questions
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
