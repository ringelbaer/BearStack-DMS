# Model smoke-test fixture

`astronaut.png` is the NASA astronaut image distributed by scikit-image 0.25.2:
https://github.com/scikit-image/scikit-image/blob/v0.25.2/skimage/data/astronaut.png

scikit-image documents the image as downloaded from NASA and public domain:
https://scikit-image.org/docs/0.25.x/api/skimage.data.html#skimage.data.astronaut

The tests check one/multiple faces, resized copies, normalized vectors and invalid
inputs. This is a functional smoke test, **not** an accuracy benchmark across ages,
lighting conditions or different identities. Automatic assignment therefore uses
conservative thresholds and always supports manual correction. No identity name
is supplied to or inferred by the engine.
