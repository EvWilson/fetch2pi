# fetch2pi

A simple utility to recursively fetch file directories hosted online and save
them to another location, in this case crafted to use for transferral to a
Raspberry Pi.

Provides the client and the server that needs to be run on the receiving
endpoint. Some elementary elements have been added regarding retries and
concurrent downloads, but not in a major or systemic manner. I've mostly put
this up to share it as a nice example of how nice it is to write a utility for
yourself to scratch an itch, and where Go really shines in creating simple
networking services.
