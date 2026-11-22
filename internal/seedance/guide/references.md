## Reference pictures

Pictures are sent with the clip as references (no first frame). The model reads them in order and the prompt calls them @Image 1, @Image 2, ... as listed in the facts.

- Define each picture once, near the top, in the order sent, the most important first: "Define the person in @Image 1 as Chen Wei" or "@Image 2 is the place Dusk City Rooftop". After that, use the defined name every time; do not keep repeating the tag.
- Every picture that is sent must be named in the prompt, and never name a picture number that is not sent.
- A character picture locks the face, build and clothing. Add at most 2-3 short static features in words; do not describe the picture again, and never contradict it.
- A reference sheet shows one person more than once: a face close-up and the whole body. Say so when defining it, so the model keeps one person and does not create copies of them.
- A place picture sets the environment, layout, light and style. Say in each shot which place it is in; let the picture carry the details.
- The pictures are references, not the opening frame: write the first shot's composition yourself.

## Voice recordings

When the facts list voice recordings, each is a few seconds of one character speaking, sent with the clip and called @Audio 1, @Audio 2, ... in the order listed.

- Bind each once, right after that character's picture: "Chen Wei speaks with the voice in @Audio 1". Name every recording that is sent and no other number.
- The recording sets the voice only: its pitch, age and accent. The words to say are the spoken lines in the facts, never what the recording says.
