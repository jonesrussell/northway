-- +goose Up
CREATE VIRTUAL TABLE article_fts USING fts5(title, body, content='articles', content_rowid='rowid');
-- +goose StatementBegin
CREATE TRIGGER articles_ai AFTER INSERT ON articles BEGIN
    INSERT INTO article_fts(rowid,title,body) VALUES(new.rowid,new.title,new.body);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER articles_ad AFTER DELETE ON articles BEGIN
    INSERT INTO article_fts(article_fts,rowid,title,body) VALUES('delete',old.rowid,old.title,old.body);
END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER articles_au AFTER UPDATE ON articles BEGIN
    INSERT INTO article_fts(article_fts,rowid,title,body) VALUES('delete',old.rowid,old.title,old.body);
    INSERT INTO article_fts(rowid,title,body) VALUES(new.rowid,new.title,new.body);
END;
-- +goose StatementEnd
INSERT INTO article_fts(article_fts) VALUES('rebuild');
