const { DataTypes } = require('sequelize');
const mongoose = require('mongoose');
const dbManager = require('../config/database');

let Token;

if (dbManager.getType() === 'mongodb') {
  const tokenSchema = new mongoose.Schema({
    name: {
      type: String,
      default: 'Default Token',
      maxlength: 50
    },
    token: {
      type: String,
      required: true,
      unique: true
    },
    userId: {
      type: mongoose.Schema.Types.ObjectId,
      ref: 'User',
      required: true
    },
    isActive: {
      type: Boolean,
      default: true
    }
  }, {
    timestamps: true
  });

  Token = mongoose.model('Token', tokenSchema);
} else {
  const initTokenModel = (sequelize) => {
    Token = sequelize.define('Token', {
      id: {
        type: DataTypes.UUID,
        defaultValue: DataTypes.UUIDV4,
        primaryKey: true
      },
      name: {
        type: DataTypes.STRING(50),
        defaultValue: 'Default Token',
        allowNull: false
      },
      token: {
        type: DataTypes.STRING,
        allowNull: false,
        unique: true
      },
      userId: {
        type: DataTypes.UUID,
        allowNull: false,
        references: {
          model: 'Users',
          key: 'id'
        }
      },
      isActive: {
        type: DataTypes.BOOLEAN,
        defaultValue: true
      }
    }, {
      timestamps: true
    });

    return Token;
  };

  Token = { initTokenModel };
}

module.exports = Token;