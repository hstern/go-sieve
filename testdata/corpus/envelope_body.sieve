require ["body", "envelope", "fileinto"];
if allof (envelope :domain "from" "newsletters.example", body :raw :contains "unsubscribe") {
	fileinto "Bulk";
}
