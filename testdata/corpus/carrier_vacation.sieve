require "vacation";
if header :contains "subject" "support request" {
	vacation "I am away until Monday.";
}
