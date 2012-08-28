gocleo
======

A golang implementation of the Cleo search.

The Cleo search is explained here: http://engineering.linkedin.com/open-source/cleo-open-source-technology-behind-linkedins-typeahead-search

The source for Jingwei Wu's version can be found here: https://github.com/linkedin/cleo

Dependencies:
gorilla mux library:  http://gorilla-web.appspot.com/pkg/mux

Instructions:
Run the program and navigate to localhost:8080/cleo/{query}

{query} is your search.  e.g.("tractor", "nightingale", "pizza")

TODO:  
1.Make port a parameter
2.Give a better explanation of the code.  
3.Split the web portion into a different file.  Perhaps "cleo_test.go".  