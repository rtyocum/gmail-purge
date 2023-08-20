# Mail Purger

You will need to create an app in the google cloud console with the gmail api enabled. Then add an OAuth key and download the json file in the same directory and remane it credentials.json. You will need the mail.google.com scope (It's restricted but can use it for personal projects & private use)
Then compile using `go build` and run the executable and let it guide you into clearing your gmail garbage. This is also very fast to the point where we are right against the google api rate limits. (A few thousand / second)