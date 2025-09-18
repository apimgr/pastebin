const { DataTypes } = require('sequelize');
const mongoose = require('mongoose');
const dbManager = require('../config/database');

let Paste;

if (dbManager.getType() === 'mongodb') {
  const pasteSchema = new mongoose.Schema({
    id: {
      type: String,
      required: true,
      unique: true
    },
    title: {
      type: String,
      maxlength: 200,
      default: 'Untitled'
    },
    content: {
      type: String,
      required: true
    },
    language: {
      type: String,
      maxlength: 50,
      default: 'text'
    },
    isPublic: {
      type: Boolean,
      default: true
    },
    expiresAt: {
      type: Date,
      default: null
    },
    userId: {
      type: mongoose.Schema.Types.ObjectId,
      ref: 'User',
      required: false
    },
    views: {
      type: Number,
      default: 0
    }
  }, {
    timestamps: true
  });

  Paste = mongoose.model('Paste', pasteSchema);
} else {
  const initPasteModel = (sequelize) => {
    Paste = sequelize.define('Paste', {
      id: {
        type: DataTypes.STRING,
        primaryKey: true
      },
      title: {
        type: DataTypes.STRING(200),
        defaultValue: 'Untitled'
      },
      content: {
        type: DataTypes.TEXT,
        allowNull: false,
        validate: {
          notEmpty: true
        }
      },
      language: {
        type: DataTypes.STRING(50),
        defaultValue: 'text'
      },
      isPublic: {
        type: DataTypes.BOOLEAN,
        defaultValue: true
      },
      expiresAt: {
        type: DataTypes.DATE,
        allowNull: true
      },
      userId: {
        type: DataTypes.UUID,
        allowNull: true,
        references: {
          model: 'Users',
          key: 'id'
        }
      },
      views: {
        type: DataTypes.INTEGER,
        defaultValue: 0
      }
    }, {
      timestamps: true
    });

    return Paste;
  };

  Paste = { initPasteModel };
}

module.exports = Paste;