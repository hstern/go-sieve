require "fileinto";
if header :contains "subject" "urgent" {
	if size :over 1048576 {
		fileinto "Big";
	} else {
		keep;
	}
} elsif exists "x-bulk" {
	discard;
}
