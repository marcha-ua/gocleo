/*
 * Copyright (c) 2011 Jacob Amrany
 * 
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 * 
 * http://www.apache.org/licenses/LICENSE-2.0
 * 
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */
package main

import (
	"bufio"
	"code.google.com/p/gorilla/mux"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

func Min(a ...int) int {
	min := int(^uint(0) >> 1) // largest int
	for _, i := range a {
		if i < min {
			min = i
		}
	}
	return min
}
func Max(a ...int) int {
	max := int(0)
	for _, i := range a {
		if i > max {
			max = i
		}
	}
	return max
}

var iIndex *InvertedIndex
var fIndex *ForwardIndex
var corpusPath string

func main() {
	flag.StringVar(&corpusPath, "Corpus_File_Path", "w1_fixed.txt", "The path to the corpus file.  A file with terms separated by \n")
	var port string
	flag.StringVar(&port, "port", "8080", "The port you want the web call to listen on.")

	iIndex = NewInvertedIndex()
	fIndex = NewForwardIndex()

	InitIndex(iIndex, fIndex)

	r := mux.NewRouter()
	r.HandleFunc("/cleo/{query}", Search)
	http.Handle("/", r)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

//Search handles the web requests and writes the output as
//json data.  
func Search(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	query := vars["query"]

	searchResult := CleoSearch(iIndex, fIndex, query)
	sort.Sort(ByScore{searchResult})
	myJson, _ := json.Marshal(searchResult)
	fmt.Fprintf(w, string(myJson))
}

func InitIndex(iIndex *InvertedIndex, fIndex *ForwardIndex) {
	//Read corpus
	file, _ := os.Open(corpusPath)

	r := bufio.NewReader(file)
	docID := 1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			break
		}
		filter := computeBloomFilter(line)

		iIndex.AddDoc(docID, line, filter) //insert into inverted index
		fIndex.AddDoc(docID, line)         //Insert into forward index

		docID++
	}
}

type RankedResults []RankedResult
type ByScore struct{ RankedResults }

func (s RankedResults) Len() int      { return len(s) }
func (s RankedResults) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s ByScore) Less(i, j int) bool  { return s.RankedResults[i].Score > s.RankedResults[j].Score }

type RankedResult struct {
	Word  string
	Score float64
}

//This is the meat of the search.  It first checks the inverted index
//for matches, then filters the potentially numerous results using
//the bloom filter.  Finally, it ranks the word using a Levenshtein
//distance.
func CleoSearch(iIndex *InvertedIndex, fIndex *ForwardIndex, query string) []RankedResult {
	t0 := time.Now()
	rslt := make([]RankedResult, 0, 0)
	fmt.Println("Query:", query)

	candidates := iIndex.Search(query) //First get candidates from Inverted Index
	qBloom := computeBloomFilter(query)

	for _, i := range candidates {
		if TestBytesFromQuery(i.bloom, qBloom) == true { //Filter using Bloom Filter
			c := fIndex.itemAt(i.docId) //Get whole document from Forward Index
			score := Score(query, c)    //Score the Forward Index between 0-1
			ranked := RankedResult{c, score}
			rslt = append(rslt, ranked)
		}
	}
	t1 := time.Now()
	fmt.Printf("The call took %v to run.\n", t1.Sub(t0))
	return rslt
}

//Iterates through all of the 8 bytes (64 bits) and tests
//each bit that is set to 1 in the query's filter against 
//the bit in the comparison's filter.  If the bit is not
// also 1, you do not have a match.
func TestBytesFromQuery(bf int, qBloom int) bool {
	for i := uint(0); i < 64; i++ {
		//a & (1 << idx) == b & (1 << idx)
		if (bf&(1<<i) != (1 << i)) && qBloom&(1<<i) == (1<<i) {
			return false
		}
	}
	return true
}

func Score(query, candidate string) float64 {
	lev := LevenshteinDistance(query, candidate)
	length := Max(len(candidate), len(query))
	return float64(length-lev) / float64(length+lev) //Jacard score
}

//Levenshtein distance is the number of inserts, deletions,
//and substitutions that differentiate one word from another.
//This algorithm is dynamic programming found at 
//http://en.wikipedia.org/wiki/Levenshtein_distance
func LevenshteinDistance(s, t string) int {
	m := len(s)
	n := len(t)
	width := n - 1
	d := make([]int, m*n)
	//y * w + h for position in array
	for i := 1; i < m; i++ {
		d[i*width+0] = i
	}

	for j := 1; j < n; j++ {
		d[0*width+j] = j
	}

	for j := 1; j < n; j++ {
		for i := 1; i < m; i++ {
			if s[i] == t[j] {
				d[i*width+j] = d[(i-1)*width+(j-1)]
			} else {
				d[i*width+j] = Min(d[(i-1)*width+j]+1, //deletion
					d[i*width+(j-1)]+1,     //insertion
					d[(i-1)*width+(j-1)]+1) //substitution
			}
		}
	}
	return d[m*(width)+0]
}

func getPrefix(query string) string {
	qLen := Min(len(query), 4)
	q := query[0:qLen]
	return strings.ToLower(q)
}

type Document struct {
	docId int
	bloom int
}

//Used for the bloom filter
const (
	FNV_BASIS_64 = uint64(14695981039346656037)
	FNV_PRIME_64 = uint64((1 << 40) + 435)
	FNV_MASK_64  = uint64(^uint64(0) >> 1)
	NUM_BITS     = 64

	FNV_BASIS_32 = uint32(0x811c9dc5)
	FNV_PRIME_32 = uint32((1 << 24) + 403)
	FNV_MASK_32  = uint32(^uint32(0) >> 1)
)

//The bloom filter of a word is 8 bytes in length
//and has each character added separately
func computeBloomFilter(s string) int {
	cnt := len(s)

	if cnt <= 0 {
		return 0
	}

	var filter int
	hash := uint64(0)

	for i := 0; i < cnt; i++ {
		c := s[i]

		//first hash function
		hash ^= uint64(0xFF & c)
		hash *= FNV_PRIME_64

		//second hash function (reduces collisions for bloom)
		hash ^= uint64(0xFF & (c >> 16))
		hash *= FNV_PRIME_64

		//position of the bit mod the number of bits (8 bytes = 64 bits)
		bitpos := hash % NUM_BITS
		if bitpos < 0 {
			bitpos += NUM_BITS
		}
		filter = filter | (1 << bitpos)
	}

	return filter
}

//Inverted Index - Maps the query prefix to the matching documents
type InvertedIndex map[string][]Document

func NewInvertedIndex() *InvertedIndex {
	i := make(InvertedIndex)
	return &i
}

func (x *InvertedIndex) Size() int {
	return len(map[string][]Document(*x))
}

func (x *InvertedIndex) AddDoc(docId int, doc string, bloom int) {
	for _, word := range strings.Fields(doc) {
		word = getPrefix(word)

		ref, ok := (*x)[word]
		if !ok {
			ref = nil
		}

		(*x)[word] = append(ref, Document{docId: docId, bloom: bloom})
	}
}

func (x *InvertedIndex) Search(query string) []Document {
	q := getPrefix(query)

	ref, ok := (*x)[q]

	if ok {
		return ref
	}
	return nil
}

//Forward Index - Maps the document id to the document
type ForwardIndex map[int]string

func NewForwardIndex() *ForwardIndex {
	i := make(ForwardIndex)
	return &i
}
func (x *ForwardIndex) AddDoc(docId int, doc string) {
	for _, word := range strings.Fields(doc) {
		_, ok := (*x)[docId]
		if !ok {
			(*x)[docId] = word
		}
	}
}
func (x *ForwardIndex) itemAt(i int) string {
	return (*x)[i]
}
