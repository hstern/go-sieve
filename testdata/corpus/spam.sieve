require ["copy", "fileinto"];
if anyof (header :contains "subject" "viagra", header :contains "subject" "$$$") {
	fileinto :copy "Junk";
	stop;
}
