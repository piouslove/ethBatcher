CREATE TABLE users
(
	id int NOT NULL AUTO_INCREMENT, 
	toAddress char(42) NOT NULL,
	value float(10, 10) NOT NULL,
	PRIMARY KEY (id)
) default charset = utf8, ENGINE=InnoDB;