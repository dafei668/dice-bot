-- 数据库迁移脚本：从单骰子升级到三骰子
-- 添加新的骰子字段

ALTER TABLE games ADD COLUMN player1_dice1 INTEGER;
ALTER TABLE games ADD COLUMN player1_dice2 INTEGER;
ALTER TABLE games ADD COLUMN player1_dice3 INTEGER;
ALTER TABLE games ADD COLUMN player2_dice1 INTEGER;
ALTER TABLE games ADD COLUMN player2_dice2 INTEGER;
ALTER TABLE games ADD COLUMN player2_dice3 INTEGER;

-- 迁移现有数据（如果有的话）
-- 将旧的单骰子数据迁移到第一个骰子字段
UPDATE games SET player1_dice1 = player1_dice WHERE player1_dice IS NOT NULL;
UPDATE games SET player2_dice1 = player2_dice WHERE player2_dice IS NOT NULL;

-- 注意：旧的 player1_dice 和 player2_dice 字段保留，以防需要回滚