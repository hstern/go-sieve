require "fileinto";
if header :contains "subject" "[list]" {
	fileinto "Lists";
}
