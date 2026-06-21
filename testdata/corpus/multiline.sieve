require "fileinto";
if header :matches "subject" "*report*" {
	fileinto text:
Reports
..hidden
.
;
}
